package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

// fileRun гоняет скрипт с журналом на «диск» и отдаёт написанное.
func fileRun(t *testing.T, script string) (*bytes.Buffer, *core.Game) {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(3).Stream("resolve")
	g := core.NewGame(*cfg)

	var file, out bytes.Buffer
	j := NewJournal(g.DB, "s1", "minimal@test", 3).WithWriter(&file)
	if err := NewSession(g, strings.NewReader(script), &out).WithJournal(j).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if err := j.Err(); err != nil {
		t.Fatalf("журнал не записался: %v", err)
	}
	return &file, g
}

// Заголовок пишется сразу: сессия, из которой не вышло ни одного хода, всё
// равно обязана быть опознаваемой.
func TestHeaderIsWrittenBeforeAnyTurn(t *testing.T) {
	var file bytes.Buffer
	NewJournal(store.NewDB(), "s1", "minimal@test", 3).WithWriter(&file)

	dump, err := ReadJournal(&file)
	if err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if dump.Session != "s1" || dump.Snapshot != "minimal@test" || dump.Seed != 3 {
		t.Errorf("заголовок неполон: %+v", dump)
	}
	if dump.CoreVersion != core.Version {
		t.Errorf("версия ядра = %q", dump.CoreVersion)
	}
	if len(dump.Commands) != 0 {
		t.Errorf("команды из ниоткуда: %d", len(dump.Commands))
	}
}

// Команда попадает в файл до отметки «применена», а не вместе с ней: файл,
// который становится читаемым только после обработки хода, не спасает от
// падения посреди хода.
func TestCommandLineIsWrittenBeforeItsAppliedMark(t *testing.T) {
	file, _ := fileRun(t, "examine body\nquit\n")
	lines := strings.Split(strings.TrimSpace(file.String()), "\n")
	var kinds []string
	for _, line := range lines {
		switch {
		case strings.Contains(line, `"kind":"header"`):
			kinds = append(kinds, "header")
		case strings.Contains(line, `"kind":"command"`):
			kinds = append(kinds, "command")
		case strings.Contains(line, `"kind":"applied"`):
			kinds = append(kinds, "applied")
		case strings.Contains(line, `"kind":"audit"`):
			kinds = append(kinds, "audit")
		}
	}
	got := strings.Join(kinds, ",")
	if got != "header,command,applied,audit" {
		t.Errorf("порядок записей = %s", got)
	}
}

// Круговорот: записанный на диск журнал читается и реплеится в то же
// состояние. Это и есть пара --journal/--replay, только без файловой системы.
func TestJournalFileReplaysIntoTheSameState(t *testing.T) {
	file, live := fileRun(t, "examine body\nrest short\ntalk_to toke\nquit\n")

	dump, err := ReadJournal(file)
	if err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if len(dump.Commands) != 3 {
		t.Fatalf("команд в файле: %d, ожидалось 3", len(dump.Commands))
	}
	for _, c := range dump.Commands {
		if c.Status != store.CommandApplied {
			t.Errorf("seq %d прочитан как %q: отметки «применена» потерялись",
				c.Seq, c.Status)
		}
	}
	if len(dump.Audit) == 0 {
		t.Error("аудит на диск не поехал")
	}

	s, replayed, _ := replayGame(t, "minimal@test", 3)
	if err := s.Replay(dump.Commands); err != nil {
		t.Fatalf("реплей: %v", err)
	}
	if got, want := fingerprint(replayed), fingerprint(live); got != want {
		t.Errorf("состояние разошлось:\nреплей %s\nпрогон %s", got, want)
	}
}

// Журнал без заголовка не реплеится: сверять снепшот и seed нечем.
func TestJournalWithoutHeaderIsRejected(t *testing.T) {
	_, err := ReadJournal(strings.NewReader(
		`{"kind":"command","command":{"seq":1}}` + "\n"))
	if err == nil {
		t.Error("журнал без заголовка прочитался молча")
	}
}

// Битая строка — ошибка с номером строки. Журнал с дырой воспроизводит не ту
// сессию, которую записывали, и пропускать дыру нельзя.
func TestBrokenLineNamesItsPlace(t *testing.T) {
	_, err := ReadJournal(strings.NewReader(
		`{"kind":"header","session":"s1"}` + "\n" + "{не json\n"))
	if err == nil {
		t.Fatal("битая строка прочиталась молча")
	}
	if !strings.Contains(err.Error(), "строка 2") {
		t.Errorf("ошибка не называет место: %v", err)
	}
}

// Неизвестный вид записи — тоже отказ: журнал завтрашней версии не
// притворяется журналом сегодняшней.
func TestUnknownRecordKindIsRejected(t *testing.T) {
	_, err := ReadJournal(strings.NewReader(
		`{"kind":"header","session":"s1"}` + "\n" + `{"kind":"world_event"}` + "\n"))
	if err == nil {
		t.Error("неизвестная запись прочиталась молча")
	}
}

// Поломка записи липкая и видна снаружи: журнал, потерявший строку, дальше не
// журнал, а на нём стоит воспроизводимость.
func TestWriteFailureIsSticky(t *testing.T) {
	j := NewJournal(store.NewDB(), "s1", "minimal@test", 3).WithWriter(brokenWriter{})
	if j.Err() == nil {
		t.Fatal("поломка записи не замечена")
	}
	if _, err := j.begin(core.Intent{Verb: "look"}, nil, 1); err == nil {
		t.Error("запись команды после поломки прошла как удачная")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("диск кончился") }
