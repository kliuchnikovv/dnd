package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func transcript(t *testing.T, seed int64, script string) string {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	g := core.NewGame(*cfg)

	in := strings.NewReader(script)
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	return out.String()
}

func TestScriptModeRunsEveryLine(t *testing.T) {
	g := renderGame(t)
	in := strings.NewReader("look\nfacts\nstate\nquit\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("прогон не дал вывода")
	}
}

func TestUnknownCommandDoesNotStopTheRun(t *testing.T) {
	g := renderGame(t)
	in := strings.NewReader("interrogate ivar\nfacts\nquit\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("неизвестная команда уронила прогон: %v", err)
	}
	if !strings.Contains(out.String(), "нельзя") {
		t.Errorf("нет сообщения об отказе: %q", out.String())
	}
}

// TestScriptedAccuseConsumesItsSlotLines регрессионный тест на общий сканер:
// Session.accuse() раньше заводил свой bufio.Scanner поверх того же s.In, что
// уже читал Run(), и это заставляло accuse() терять все четыре строки со
// значениями слотов (первый Scan() в Run() вычерпывал остаток скрипта в свой
// внутренний буфер). Скрипт ниже гонит `accuse` через реальный REPL-цикл и
// проверяет, что все четыре ответа дошли и обвинение подтвердилось.
func TestScriptedAccuseConsumesItsSlotLines(t *testing.T) {
	g := renderGame(t)
	in := strings.NewReader("accuse\ntoke\ncord\nnight\naudit\nquit\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Обвинение верно.") {
		t.Errorf("верное обвинение не распознано: %q", got)
	}
	for _, slot := range []string{"who:", "how:", "when:", "why:"} {
		if !strings.Contains(got, slot) {
			t.Errorf("слот %q не был запрошен: %q", slot, got)
		}
	}
}

func TestSameSeedSameTranscript(t *testing.T) {
	// Пара (seed, скрипт) полностью задаёт вывод — это и есть харнесс.
	script := "look\nexamine body\nfacts\nquit\n"
	first := transcript(t, 7, script)
	second := transcript(t, 7, script)
	if first != second {
		t.Error("один seed дал разные транскрипты")
	}
	if other := transcript(t, 8, script); other == first {
		t.Log("разные seed дали одинаковый транскрипт — допустимо на коротком скрипте")
	}
}
