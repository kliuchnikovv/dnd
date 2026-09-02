package e2e

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func gameFor(t *testing.T, casePath string, seed int64) *core.Game {
	t.Helper()
	cfg, err := cases.Load(casePath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	return core.NewGame(*withActorCharacter(cfg))
}

func runForte(t *testing.T, seed int64) string {
	t.Helper()
	script, err := os.ReadFile("../cases/forte_merlo/walkthrough.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	g := gameFor(t, "../cases/forte_merlo/case.json", seed)
	if err := cli.NewSession(g, bytes.NewReader(script), &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	return out.String()
}

func TestForteMerloWalkthroughReachesCorrectAccusation(t *testing.T) {
	out := runForte(t, 3)
	if !strings.Contains(out, "Обвинение верно") {
		t.Fatalf("прохождение не дошло до верного обвинения:\n%s", out)
	}
}

func TestForteMerloWalkthroughIsReproducible(t *testing.T) {
	if runForte(t, 3) != runForte(t, 3) {
		t.Error("один seed и один скрипт дали разные транскрипты")
	}
}

// Порог 2-из-3 и оба вывода через compare — те механики, ради которых
// расширялся движок. Если они перестанут срабатывать, дело останется
// проходимым по другим путям, и молчаливая деградация пройдёт незамеченной.
func TestForteMerloExercisesThresholdAndReasoning(t *testing.T) {
	out := runForte(t, 3)
	for _, want := range []string{
		"strangled with his own curtain",  // факт за порогом 2 из 3
		"matches the break of the shards", // вывод через compare
		"between 23:30 and 03:00",         // второй вывод через compare
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в прогоне нет %q — механика не сработала", want)
		}
	}
}

// Ни один вывод не должен раскрывать правильный ответ до обвинения.
func TestForteMerloNeverPrintsTruth(t *testing.T) {
	out := runForte(t, 3)
	if strings.Contains(out, "<redacted>") {
		t.Error("в выводе засветился редактированный truth")
	}
}
