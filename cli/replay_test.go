package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

// replayGame — свежая сессия того же дела на том же seed. Реплей стартует
// именно с этого: снепшот прототипа это дело плюс seed.
func replayGame(t *testing.T, snapshot string, seed int64) (*Session, *core.Game, *bytes.Buffer) {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	g := core.NewGame(*withActorCharacter(cfg))

	var out bytes.Buffer
	s := NewSession(g, strings.NewReader(""), &out).
		WithJournal(NewJournal(g.DB, "s1", snapshot, seed))
	return s, g, &out
}

// fingerprint — отпечаток состояния сессии. Реплей обязан привести к тому же
// состоянию, а не к похожему выводу.
func fingerprint(g *core.Game) string {
	var b strings.Builder
	for _, f := range g.K.TopicBank() {
		if g.K.Knows(f) {
			b.WriteString(string(f) + " ")
		}
	}
	b.WriteString("| " + string(g.Node))
	for _, c := range g.C.Snapshot() {
		b.WriteString(" | " + string(c.ID) + ":")
		b.WriteString(string(rune('0' + c.Filled)))
	}
	if g.Solved() {
		b.WriteString(" | solved")
	}
	return b.String()
}

// Записанная сессия реплеится в идентичное состояние. Это исполнимая форма
// контракта ADR-0001: (снепшот, seed, журнал) воспроизводит сессию.
func TestReplayReproducesState(t *testing.T) {
	live := journalRun(t, "examine body\nrest short\ntalk_to toke\nquit\n")
	entries := live.Commands("s1")
	if len(entries) != 3 {
		t.Fatalf("записано команд: %d, ожидалось 3", len(entries))
	}

	replayed, g, _ := replayGame(t, "minimal@test", 3)
	if err := replayed.Replay(entries); err != nil {
		t.Fatalf("реплей: %v", err)
	}

	liveGame := journalGame(t, "examine body\nrest short\ntalk_to toke\nquit\n")
	if got, want := fingerprint(g), fingerprint(liveGame); got != want {
		t.Errorf("состояние разошлось:\nреплей %s\nпрогон %s", got, want)
	}
}

// Реплей не пишет журнал второй раз: ключ идемпотентности узнаёт уже
// записанную команду. Иначе каждый прогон журнала удваивал бы его.
func TestReplayDoesNotDuplicateTheJournal(t *testing.T) {
	live := journalRun(t, "examine body\nrest short\nquit\n")
	before := len(live.CommandLog)

	// Проверяется свойство самого журнала, поэтому база берётся та же:
	// состояние здесь не при чём, важно, что строк не прибавилось.
	s := sessionOver(t, live, "minimal@test", 3)
	if err := s.Replay(live.Commands("s1")); err != nil {
		t.Fatalf("реплей: %v", err)
	}
	if after := len(live.CommandLog); after != before {
		t.Errorf("журнал вырос при реплее: было %d, стало %d", before, after)
	}
}

// Хвост pending — прерванный ход. Реплей доигрывает его и отмечает
// применённым ровно один раз.
func TestPendingTailIsReplayedOnce(t *testing.T) {
	live := journalRun(t, "examine body\nquit\n")
	// Падение между «записал» и «применил»: команда есть, статуса applied нет.
	interrupted, appended := live.AppendCommand(store.CommandLogEntry{
		SessionID:      "s1",
		SnapshotID:     "minimal@test",
		CoreVersion:    core.Version,
		Intent:         []byte(`{"verb":"rest","args":{"text":"short"}}`),
		DiceCtx:        store.DiceCtx{Seed: 3, Turn: 2},
		IdempotencyKey: "interrupted",
	})
	if !appended {
		t.Fatal("прерванный ход не записан")
	}
	if n := len(live.PendingTail("s1")); n != 1 {
		t.Fatalf("хвост pending: %d, ожидался 1", n)
	}

	s := sessionOver(t, live, "minimal@test", 3)
	before := len(live.CommandLog)
	if err := s.Replay(live.Commands("s1")); err != nil {
		t.Fatalf("реплей: %v", err)
	}
	if after := len(live.CommandLog); after != before {
		t.Errorf("реплей записал команды заново: было %d, стало %d", before, after)
	}
	if n := len(live.PendingTail("s1")); n != 0 {
		t.Errorf("хвост pending остался после реплея: %d", n)
	}
	if got := live.Commands("s1")[interrupted.Seq-1].Status; got != store.CommandApplied {
		t.Errorf("прерванный ход не отмечен применённым: %q", got)
	}
}

// Расхождение версии ядра — явная ошибка. Правка правил меняет исход прошлых
// бросков, и молчаливо выдать другую сессию за ту же нельзя.
func TestReplayRejectsCoreVersionMismatch(t *testing.T) {
	live := journalRun(t, "examine body\nquit\n")
	entries := live.Commands("s1")
	entries[0].CoreVersion = "core-0"

	s, _, _ := replayGame(t, "minimal@test", 3)
	err := s.Replay(entries)
	if err == nil {
		t.Fatal("реплей на другой версии ядра прошёл молча")
	}
	if !strings.Contains(err.Error(), "core-0") {
		t.Errorf("ошибка не называет версию журнала: %v", err)
	}
}

// Чужой снепшот и чужой seed — тоже отказ: журнал одной сессии не является
// журналом другой, даже если команды в нём исполнимы.
func TestReplayRejectsForeignSnapshotAndSeed(t *testing.T) {
	live := journalRun(t, "examine body\nquit\n")

	s, _, _ := replayGame(t, "harbour@other", 3)
	if err := s.Replay(live.Commands("s1")); err == nil {
		t.Error("реплей чужого снепшота прошёл молча")
	}

	s, _, _ = replayGame(t, "minimal@test", 7)
	if err := s.Replay(live.Commands("s1")); err == nil {
		t.Error("реплей на чужом seed прошёл молча")
	}
}

// Реплей без журнала отказывает: сверять снепшот и seed нечем, а реплей без
// сверки воспроизводит не сессию, а совпадение.
func TestReplayWithoutJournalRefuses(t *testing.T) {
	g := renderGame(t)
	s := NewSession(g, strings.NewReader(""), &bytes.Buffer{})
	if err := s.Replay(nil); err != ErrNoJournal {
		t.Errorf("ошибка = %v, ожидалась ErrNoJournal", err)
	}
}

// Битая запись роняет реплей с указанием места, а не тихо пропускается.
func TestReplayFailsLoudlyOnBrokenEntry(t *testing.T) {
	s, _, _ := replayGame(t, "minimal@test", 3)
	err := s.Replay([]store.CommandLogEntry{{
		Seq: 4, SnapshotID: "minimal@test", CoreVersion: core.Version,
		DiceCtx: store.DiceCtx{Seed: 3, Turn: 1},
		Intent:  []byte(`{"verb":"accuse"}`),
	}})
	if err == nil {
		t.Fatal("обвинение без формы прошло молча")
	}
	if !strings.Contains(err.Error(), "seq 4") {
		t.Errorf("ошибка не называет место: %v", err)
	}
}
