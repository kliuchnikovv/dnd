package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

// journalRun гоняет скрипт через реальный цикл хода с включённым журналом и
// отдаёт базу: журнал живёт в тех же таблицах, что и остальное состояние.
func journalRun(t *testing.T, script string) *store.DB {
	t.Helper()
	db, _ := journalSession(t, script, nil)
	return db
}

// journalSession собирает сессию поверх дела-минимума. spy, если он задан,
// подменяет систему правил: только так видно, что журнал пишется ДО ядра.
func journalSession(t *testing.T, script string, spy core.RuleSystem) (*store.DB, string) {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	if spy != nil {
		cfg.Rules = spy
	}
	cfg.Dice = dice.NewSource(3).Stream("resolve")
	g := core.NewGame(*withActorCharacter(cfg))

	var out bytes.Buffer
	s := NewSession(g, strings.NewReader(script), &out).
		WithJournal(NewJournal(g.DB, "s1", "minimal@test", 3))
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	return g.DB, out.String()
}

// beforeApply — система правил, подсматривающая журнал в момент броска.
// Бросок происходит внутри core.Apply, поэтому увиденное здесь — это то, что
// лежало в журнале ДО обработки хода.
type beforeApply struct {
	inner core.RuleSystem
	db    *store.DB
	seen  []store.CommandLogEntry
}

func (b *beforeApply) Resolve(in core.Intent, v core.SceneView, d core.Dice) core.Resolution {
	b.seen = append(b.seen, b.db.Commands("s1")...)
	return b.inner.Resolve(in, v, d)
}

func (b *beforeApply) PassiveScore(v core.SceneView) int { return b.inner.PassiveScore(v) }

// Главный инвариант фазы: команда лежит в журнале как pending раньше, чем
// ядро её обработало. Падение между «записал» и «применил» лечится реплеем
// хвоста — но только если хвост успел появиться.
func TestCommandIsPendingBeforeCoreApplies(t *testing.T) {
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	spy := &beforeApply{inner: threshold.New()}
	cfg.Rules = spy
	cfg.Dice = dice.NewSource(3).Stream("resolve")
	g := core.NewGame(*withActorCharacter(cfg))
	spy.db = g.DB

	var out bytes.Buffer
	s := NewSession(g, strings.NewReader("examine body\nquit\n"), &out).
		WithJournal(NewJournal(g.DB, "s1", "minimal@test", 3))
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}

	if len(spy.seen) == 0 {
		t.Fatal("бросок не случился — тест ничего не проверил")
	}
	if len(spy.seen) != 1 {
		t.Fatalf("в момент броска в журнале %d команд, ожидалась 1", len(spy.seen))
	}
	if spy.seen[0].Status != store.CommandPending {
		t.Errorf("статус в момент броска = %q, ожидался %q",
			spy.seen[0].Status, store.CommandPending)
	}
}

// После хода команда отмечена применённой: журнал не оставляет применённый ход
// висеть в хвосте, иначе реплей применит его второй раз.
func TestCommandIsAppliedAfterTurn(t *testing.T) {
	db := journalRun(t, "examine body\nquit\n")
	cmds := db.Commands("s1")
	if len(cmds) != 1 {
		t.Fatalf("команд в журнале: %d, ожидалась 1", len(cmds))
	}
	if cmds[0].Status != store.CommandApplied {
		t.Errorf("статус после хода = %q, ожидался %q", cmds[0].Status, store.CommandApplied)
	}
	if n := len(db.PendingTail("s1")); n != 0 {
		t.Errorf("хвост pending непуст после завершённого хода: %d", n)
	}
}

// Порядок seq — это порядок ходов: реплей воспроизводит сессию по нему.
func TestSeqFollowsTurnOrder(t *testing.T) {
	db := journalRun(t, "examine body\nsearch quay\nlook\nquit\n")
	cmds := db.Commands("s1")
	if len(cmds) != 3 {
		t.Fatalf("команд в журнале: %d, ожидалось 3", len(cmds))
	}
	verbs := []core.Verb{"examine", "search", "look"}
	for i, want := range verbs {
		if cmds[i].Seq != i+1 {
			t.Errorf("%d: seq = %d", i, cmds[i].Seq)
		}
		if got := journaledVerb(t, cmds[i]); got != want {
			t.Errorf("%d: глагол = %q, ожидался %q", i, got, want)
		}
		if cmds[i].DiceCtx.Turn != i+1 {
			t.Errorf("%d: номер хода = %d", i, cmds[i].DiceCtx.Turn)
		}
	}
}

// Справки и свободные пробы состояние не меняют и в журнал команд не идут:
// журнал — правда реплея, а не лог нажатий.
func TestFreeQueriesAreNotJournaled(t *testing.T) {
	db := journalRun(t, "survey\nfacts\nstate\nitems\nclocks\nhelp\nquit\n")
	if n := len(db.Commands("s1")); n != 0 {
		t.Errorf("справки попали в журнал: %d команд", n)
	}
}

// Отдых и сопоставление меняют состояние (часы и выведенные факты), и без них
// реплей журнала разойдётся с прогоном.
func TestStateChangingNonIntentTurnsAreJournaled(t *testing.T) {
	db := journalRun(t, "rest short\ncompare f_ligature f_ligature\nquit\n")
	cmds := db.Commands("s1")
	if len(cmds) != 2 {
		t.Fatalf("команд в журнале: %d, ожидалось 2", len(cmds))
	}
	if got := journaledVerb(t, cmds[0]); got != "rest" {
		t.Errorf("отдых записан как %q", got)
	}
	if got := journaledVerb(t, cmds[1]); got != "compare" {
		t.Errorf("сопоставление записано как %q", got)
	}
	var decoded struct {
		Args struct {
			Facts []store.FactID `json:"facts"`
		} `json:"args"`
	}
	if err := json.Unmarshal(cmds[1].Intent, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Args.Facts) != 2 {
		t.Errorf("сопоставление без пары фактов: %v", decoded.Args.Facts)
	}
}

// Обвинение — ход, заканчивающий дело, и в журнале лежит вместе с формой:
// без неё реплей не сможет его повторить.
func TestAccusationIsJournaledWithItsForm(t *testing.T) {
	db := journalRun(t, "accuse\ntoke\ncord\nnight\naudit\nquit\n")
	cmds := db.Commands("s1")
	if len(cmds) != 1 {
		t.Fatalf("команд в журнале: %d, ожидалась 1", len(cmds))
	}
	if got := journaledVerb(t, cmds[0]); got != "accuse" {
		t.Errorf("обвинение записано как %q", got)
	}
	var decoded struct {
		Form *struct {
			Who string `json:"who"`
			Why string `json:"why"`
		} `json:"form"`
	}
	if err := json.Unmarshal(cmds[0].Intent, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Form == nil {
		t.Fatal("форма обвинения не записана")
	}
	if decoded.Form.Who != "toke" || decoded.Form.Why != "audit" {
		t.Errorf("форма записана неверно: %+v", *decoded.Form)
	}
}

// Отказ мира — тоже обработанный ход: запись до обработки не умеет знать
// заранее, что ядро откажет, и оставлять такую команду в хвосте нельзя.
func TestRefusedTurnIsJournaledAndApplied(t *testing.T) {
	db, out := journalSession(t, "question ghost\nquit\n", nil)
	if !strings.Contains(out, "нельзя") {
		t.Fatalf("ход не был отказан, тест ничего не проверил: %q", out)
	}
	cmds := db.Commands("s1")
	if len(cmds) != 1 {
		t.Fatalf("команд в журнале: %d, ожидалась 1", len(cmds))
	}
	if cmds[0].Status != store.CommandApplied {
		t.Errorf("отказанный ход остался %q", cmds[0].Status)
	}
}

// Контекст детерминизма и снепшот пишутся в каждую строку: без них журнал не
// воспроизводит кость, а значит не воспроизводит и сессию.
func TestEveryRowCarriesDeterminismContext(t *testing.T) {
	db := journalRun(t, "examine body\nquit\n")
	e := db.Commands("s1")[0]
	if e.DiceCtx.Seed != 3 {
		t.Errorf("seed = %d, ожидался 3", e.DiceCtx.Seed)
	}
	if e.SnapshotID != "minimal@test" {
		t.Errorf("снепшот = %q", e.SnapshotID)
	}
	if e.CoreVersion == "" {
		t.Error("версия ядра не записана — реплей не сможет её сверить")
	}
	if e.IdempotencyKey == "" {
		t.Error("ключ идемпотентности не записан — реплей применит ход дважды")
	}
}

// Ключ идемпотентности детерминирован: тот же прогон даёт те же ключи, иначе
// реплей не узнает уже применённую команду.
func TestIdempotencyKeyIsDeterministic(t *testing.T) {
	script := "examine body\nsearch quay\nquit\n"
	first, second := journalRun(t, script), journalRun(t, script)
	a, b := first.Commands("s1"), second.Commands("s1")
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("прогоны дали разное число команд: %d и %d", len(a), len(b))
	}
	for i := range a {
		if a[i].IdempotencyKey != b[i].IdempotencyKey {
			t.Errorf("%d: ключи разошлись: %q и %q",
				i, a[i].IdempotencyKey, b[i].IdempotencyKey)
		}
	}
	if a[0].IdempotencyKey == a[1].IdempotencyKey {
		t.Error("два разных хода получили один ключ")
	}
}

// Сессия без журнала не пишет ничего и печатает то же самое: игра без
// надстроек обязана работать как работала.
func TestSessionWithoutJournalWritesNothing(t *testing.T) {
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(3).Stream("resolve")
	g := core.NewGame(*withActorCharacter(cfg))

	var out bytes.Buffer
	if err := NewSession(g, strings.NewReader("examine body\nquit\n"), &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if n := len(g.DB.CommandLog); n != 0 {
		t.Errorf("журнал заполнился без просьбы: %d строк", n)
	}

	_, journaled := journalSession(t, "examine body\nquit\n", nil)
	if journaled != out.String() {
		t.Errorf("журнал изменил вывод игры:\nбез: %q\nс:   %q", out.String(), journaled)
	}
}

// journaledVerb достаёт глагол из jsonb-колонки команды.
func journaledVerb(t *testing.T, e store.CommandLogEntry) core.Verb {
	t.Helper()
	var decoded struct {
		Verb core.Verb `json:"verb"`
	}
	if err := json.Unmarshal(e.Intent, &decoded); err != nil {
		t.Fatalf("разбор интента: %v", err)
	}
	return decoded.Verb
}

// journalGame — тот же прогон, что journalRun, но отдаёт игру: состояние нужно
// сравнивать с состоянием, а не с выводом.
func journalGame(t *testing.T, script string) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(3).Stream("resolve")
	g := core.NewGame(*withActorCharacter(cfg))

	var out bytes.Buffer
	s := NewSession(g, strings.NewReader(script), &out).
		WithJournal(NewJournal(g.DB, "s1", "minimal@test", 3))
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	return g
}

// sessionOver — сессия поверх той же базы, в которую журнал записан. Годится
// только для проверок о самом журнале: состояние в этой базе уже отыграно, и
// сравнивать его после реплея бессмысленно (для этого — replayGame и e2e).
func sessionOver(t *testing.T, db *store.DB, snapshot string, seed int64) *Session {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	cfg.DB = db
	g := core.NewGame(*withActorCharacter(cfg))
	return NewSession(g, strings.NewReader(""), &bytes.Buffer{}).
		WithJournal(NewJournal(db, "s1", snapshot, seed))
}
