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

func newGame(t *testing.T, seed int64) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	return core.NewGame(*cfg)
}

func TestWalkthroughReachesCorrectAccusation(t *testing.T) {
	script, err := os.ReadFile("../cases/harbour/walkthrough.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	g := newGame(t, 3)
	if err := cli.NewSession(g, bytes.NewReader(script), &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if !strings.Contains(out.String(), "Обвинение верно") {
		t.Fatalf("прохождение не дошло до верного обвинения:\n%s", out.String())
	}
}

func TestWalkthroughIsReproducible(t *testing.T) {
	script, _ := os.ReadFile("../cases/harbour/walkthrough.txt")
	run := func() string {
		var out bytes.Buffer
		cli.NewSession(newGame(t, 3), bytes.NewReader(script), &out).Run()
		return out.String()
	}
	if run() != run() {
		t.Error("один seed и один скрипт дали разные транскрипты")
	}
}
