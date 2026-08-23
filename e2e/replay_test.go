package e2e

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
)

// snapshotID — снепшот прототипа: дело плюс seed. Реплей стартует с него, и
// журнал одной сессии не является журналом другой.
const snapshotID = "harbour@3"

// fingerprint — отпечаток состояния сессии. Реплей обязан привести к тому же
// состоянию, а не к похожему выводу: транскрипты и не должны совпадать, потому
// что справки (survey, facts) состояние не меняют и в журнал не идут.
func fingerprint(g *core.Game) string {
	var known []string
	for _, f := range g.K.TopicBank() {
		if g.K.Knows(f) {
			known = append(known, string(f))
		}
	}
	sort.Strings(known)
	var clocks []string
	for _, c := range g.C.Snapshot() {
		clocks = append(clocks, fmt.Sprintf("%s:%d", c.ID, c.Filled))
	}
	return fmt.Sprintf("facts[%s] node=%s clocks[%s] solved=%v attempts=%d",
		strings.Join(known, ","), g.Node, strings.Join(clocks, ","),
		g.Solved(), g.Attempts)
}

// Обобщение walkthrough-теста на журнал: прохождение «Гавани» записывается, и
// записанный журнал воспроизводит ту же сессию. Это исполнимая форма контракта
// ADR-0001 — раньше он существовал на бумаге, потому что script был файлом, а
// не сущностью.
func TestRecordedWalkthroughReplaysIntoTheSameState(t *testing.T) {
	script, err := os.ReadFile("../cases/harbour/walkthrough.txt")
	if err != nil {
		t.Fatal(err)
	}

	live := newGame(t, 3)
	var liveOut bytes.Buffer
	err = cli.NewSession(live, bytes.NewReader(script), &liveOut).
		WithJournal(cli.NewJournal(live.DB, "walkthrough", snapshotID, 3)).
		Run()
	if err != nil {
		t.Fatalf("живой прогон: %v", err)
	}
	if !strings.Contains(liveOut.String(), "Обвинение верно") {
		t.Fatal("живой прогон не дошёл до верного обвинения — реплеить нечего")
	}
	entries := live.DB.Commands("walkthrough")
	if len(entries) == 0 {
		t.Fatal("журнал пуст")
	}

	// Реплей: свежее дело, тот же seed, журнал вместо ввода игрока. Сырых
	// фраз игрока здесь нет вовсе — правда реплея это интент.
	replayed := newGame(t, 3)
	var replayOut bytes.Buffer
	s := cli.NewSession(replayed, strings.NewReader(""), &replayOut).
		WithJournal(cli.NewJournal(replayed.DB, "walkthrough", snapshotID, 3))
	if err := s.Replay(entries); err != nil {
		t.Fatalf("реплей: %v", err)
	}

	if got, want := fingerprint(replayed), fingerprint(live); got != want {
		t.Errorf("состояние разошлось:\nреплей %s\nпрогон %s", got, want)
	}
	if !replayed.Solved() {
		t.Error("реплей не закрыл дело, которое живой прогон закрыл")
	}
	if !strings.Contains(replayOut.String(), "Обвинение верно") {
		t.Errorf("реплей не дошёл до верного обвинения:\n%s", replayOut.String())
	}
}

// Журнал худой: он хранит ходы, меняющие состояние, а не каждую строку ввода.
// Справки в него не идут, поэтому команд в нём заведомо меньше, чем строк в
// скрипте, — и на состояние это не влияет (проверено тестом выше).
func TestJournalIsThinnerThanTheScript(t *testing.T) {
	script, err := os.ReadFile("../cases/harbour/walkthrough.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, line := range strings.Split(string(script), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines++
		}
	}

	g := newGame(t, 3)
	var out bytes.Buffer
	if err := cli.NewSession(g, bytes.NewReader(script), &out).
		WithJournal(cli.NewJournal(g.DB, "walkthrough", snapshotID, 3)).
		Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	cmds := len(g.DB.Commands("walkthrough"))
	if cmds == 0 || cmds >= lines {
		t.Errorf("команд в журнале %d при %d строках ввода — журнал не худой",
			cmds, lines)
	}
}
