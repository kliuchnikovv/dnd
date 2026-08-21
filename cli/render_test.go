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

// survey — единственный механизм под критерий гейта «понятно ли, что делать
// дальше, без подсказки». Список обязан быть честным: холдеры и пропы в одном
// перечне, без разметки. Размеченный список — это карта решения.
func TestSurveyListsPropsAndHoldersWithoutMarking(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Survey(g)

	for _, want := range []string{"Тело Халдена", "Ящики у стены", "Погасший фонарь"} {
		if !strings.Contains(out, want) {
			t.Errorf("в survey нет цели %q: %q", want, out)
		}
	}

	prefixes := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "·") {
			continue
		}
		prefixes[line[:strings.Index(line, "·")+len("·")]] = true
	}
	if len(prefixes) != 1 {
		t.Errorf("цели размечены по-разному, список выдаёт граф: %q", out)
	}
}

// Порядок произвольный, но стабильный: скриптовый прогон обязан быть
// воспроизводим, а игрок не должен видеть, как список перетасовывается.
func TestSurveyOrderIsStable(t *testing.T) {
	g := renderGame(t)
	first := Render{}.Survey(g)
	for i := 0; i < 20; i++ {
		got := Render{}.Survey(g)
		if got != first {
			t.Fatalf("порядок survey поплыл на прогоне %d:\n%q\n%q", i, first, got)
		}
	}
}

// Безопасный переход кости не трогает, и печатать «[d20=0 против 0]» значит
// показывать игроку бросок, которого не было.
func TestNoRollNoRollLine(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Turn(g, core.TurnResult{Res: &core.Resolution{Class: core.OutcomeSuccess}})
	if strings.Contains(out, "d20") {
		t.Errorf("напечатан несуществующий бросок: %q", out)
	}
}

// Цена провала обязана быть видна в тот же ход. Тик, заметный только когда
// часы заполнятся, — это не цена, а сюрприз через двадцать минут.
func TestCostIsShownTheSameTurn(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Turn(g, core.TurnResult{
		Res:   &core.Resolution{Class: core.OutcomeFail, Log: core.RollLog{Die: 4}},
		Costs: []core.CostKind{core.CostTickClock, core.CostDispositionDown},
	})
	for _, want := range []string{"часы", "расположение"} {
		if !strings.Contains(out, want) {
			t.Errorf("цена %q не показана: %q", want, out)
		}
	}
}

func TestSuccessShowsNoCostLine(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Turn(g, core.TurnResult{
		Res: &core.Resolution{Class: core.OutcomeSuccess, Log: core.RollLog{Die: 18}},
	})
	if strings.Contains(out, "цена") {
		t.Errorf("у успеха появилась цена: %q", out)
	}
}
