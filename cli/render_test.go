package cli

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func renderGame(t *testing.T) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	return core.NewGame(*cfg)
}

func refusedResult(msg string) core.TurnResult {
	return core.TurnResult{Refused: true, Refusal: msg}
}

func TestRefusalReadsDifferentlyFromFailure(t *testing.T) {
	// Игрок обязан мгновенно видеть разницу: отказ не потратил ход.
	g := renderGame(t)
	r := Render{}
	refusal := r.Turn(g, refusedResult("парти об этом ничего не знает"))
	if !strings.Contains(refusal, "нельзя") {
		t.Errorf("отказ не помечен как отказ: %q", refusal)
	}
	if strings.Contains(refusal, "ПРОВАЛ") {
		t.Errorf("отказ подан как провал: %q", refusal)
	}
}

func TestFactsShowSourcesAndConfidence(t *testing.T) {
	g := renderGame(t)
	g.K.Learn("f_ligature", "e_body")
	g.K.Learn("f_ligature", "e_toke")
	out := Render{}.Facts(g)
	if !strings.Contains(out, "0.75") {
		t.Errorf("confidence не показан: %q", out)
	}
	if !strings.Contains(out, "e_body") || !strings.Contains(out, "e_toke") {
		t.Errorf("источники не показаны: %q", out)
	}
}

func TestStateShowsAttemptsAndGrit(t *testing.T) {
	g := renderGame(t)
	out := Render{}.State(g)
	for _, want := range []string{"узел", "grit", "попыт"} {
		if !strings.Contains(strings.ToLower(out), want) {
			t.Errorf("в state нет %q: %q", want, out)
		}
	}
}

func TestRenderNeverPrintsTruth(t *testing.T) {
	g := renderGame(t)
	all := Render{}.Scene(g) + Render{}.Facts(g) + Render{}.State(g) + Render{}.Clocks(g)
	for _, leak := range []string{"toke_is_killer", "<redacted>"} {
		if strings.Contains(all, leak) {
			t.Errorf("вывод содержит %q", leak)
		}
	}
}

func TestHelpListsEveryPlayerCommand(t *testing.T) {
	out := Render{}.Help()
	for _, cmd := range []string{"question", "examine", "search", "compare",
		"cross_reference", "stake_out", "theorize", "accuse", "facts", "state", "clocks"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("help не упоминает %q", cmd)
		}
	}
}
