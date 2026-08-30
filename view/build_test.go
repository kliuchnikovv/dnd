package view

import (
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

// Диалоговые вербы получают человеческий лейбл, а не сырой «глагол цель».
func TestAffordanceLabelForConversationalVerbs(t *testing.T) {
	g := buildGame(t)
	cases := []struct {
		verb  core.Verb
		check core.VerbClass
		want  string
	}{
		{"ask_about", core.ClassSocial, "поговорить о другом [общение]"},
		{"threaten_verbally", core.ClassSocial, "надавить [общение]"},
	}
	for _, c := range cases {
		a := core.Affordance{Intent: core.Intent{Verb: c.verb, Args: core.Args{Target: "e_bern"}}, Check: c.check}
		if got := AffordanceLabel(g, a); got != c.want {
			t.Errorf("%s → %q, ждали %q", c.verb, got, c.want)
		}
	}
}

func buildGame(t *testing.T) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	return core.NewGame(*cfg)
}

// fakeRuleset и fakeScenario — оси, наполненные СЕНТИНЕЛАМИ. Сборщик не имеет
// права называть меру или панель сам: назвал бы — шов протёк, и это ловится
// тем, что в тесте метки заведомо не те, что у детектива.
type fakeRuleset struct{ meters []Meter }

func (f fakeRuleset) Meters(*core.Game) []Meter { return f.meters }

type fakeScenario struct{ panel *Panel }

func (f fakeScenario) Objective(*core.Game) *Panel { return f.panel }

var noRule = fakeRuleset{}
var noScenario = fakeScenario{}

// TestBuildFillsSceneFromCore — сцену ставит ядро: узел и его имя из базы, а не
// из осей. Это агностичная часть вида — она одинакова у любого правила и
// сценария.
func TestBuildFillsSceneFromCore(t *testing.T) {
	g := buildGame(t)
	tv := Build(g, core.TurnResult{}, nil, noRule, noScenario, "")
	if tv.Scene.Node != "n_quay" || tv.Scene.Title != "Пристань" {
		t.Errorf("сцена не из ядра: %+v", tv.Scene)
	}
}

// TestBuildDelegatesMetersToRuleset — меры приходят ЦЕЛИКОМ от правила. Сборщик
// их не сочиняет и не фильтрует: surface-решение тоже на правиле.
func TestBuildDelegatesMetersToRuleset(t *testing.T) {
	g := buildGame(t)
	rs := fakeRuleset{meters: []Meter{{Label: "сентинель", Kind: "x", Value: 7, Surface: true}}}
	tv := Build(g, core.TurnResult{}, nil, rs, noScenario, "")
	if len(tv.Meters) != 1 || tv.Meters[0].Label != "сентинель" || tv.Meters[0].Value != 7 {
		t.Errorf("меры не от правила: %+v", tv.Meters)
	}
}

// TestBuildDelegatesObjectiveToScenario — панель цели приходит от сценария и
// проносится как есть.
func TestBuildDelegatesObjectiveToScenario(t *testing.T) {
	g := buildGame(t)
	sc := fakeScenario{panel: &Panel{Kind: "sentinel", Title: "П"}}
	tv := Build(g, core.TurnResult{}, nil, noRule, sc, "")
	if tv.Objective == nil || tv.Objective.Kind != "sentinel" {
		t.Errorf("панель не от сценария: %+v", tv.Objective)
	}
}

// TestBuildMapsResolution — d20-бросок «Порога» приходит в вид как термы+цель+
// исход, без словаря кости. Tier — машинная ступень для клиента, Label — слово
// игроку.
func TestBuildMapsResolution(t *testing.T) {
	g := buildGame(t)
	res := core.TurnResult{Res: &core.Resolution{
		Class:  core.OutcomeSuccess,
		Margin: 3,
		Log: core.RollLog{Die: 20, Threshold: 12,
			Terms: []core.RollTerm{{Name: "расследование", Value: 2}}},
	}}
	tv := Build(g, res, nil, noRule, noScenario, "")
	if tv.Resolution == nil {
		t.Fatal("резолюция потерялась")
	}
	r := tv.Resolution
	if r.Target != 12 || r.Margin != 3 {
		t.Errorf("цель/маржа: %+v", r)
	}
	if len(r.Terms) != 1 || r.Terms[0].Label != "расследование" || r.Terms[0].Value != 2 {
		t.Errorf("термы: %+v", r.Terms)
	}
	if r.Outcome.Tier != "success" || r.Outcome.Label != core.OutcomeSuccess.String() {
		t.Errorf("исход: %+v", r.Outcome)
	}
}

// TestBuildNoResolutionWhenNoRoll — у безопасного действия броска нет, и вид не
// несёт пустую резолюцию.
func TestBuildNoResolutionWhenNoRoll(t *testing.T) {
	g := buildGame(t)
	tv := Build(g, core.TurnResult{}, nil, noRule, noScenario, "")
	if tv.Resolution != nil {
		t.Errorf("резолюция взялась из ниоткуда: %+v", tv.Resolution)
	}
}

// TestBuildOptionsFromAffordances — опции строятся из read-scope аффордансов
// ядра, со словами и с сохранением признака реплики.
func TestBuildOptionsFromAffordances(t *testing.T) {
	g := buildGame(t)
	tv := Build(g, core.TurnResult{}, nil, noRule, noScenario, "")
	if len(tv.Options) == 0 {
		t.Fatal("на старте есть, что предложить, но опций нет")
	}
	if len(tv.Options) != len(g.Affordances("")) {
		t.Errorf("опций %d, аффордансов %d", len(tv.Options), len(g.Affordances("")))
	}
	for i, o := range tv.Options {
		if o.Label == "" {
			t.Errorf("опция %d без слов", i)
		}
	}
}

// TestBuildParticipantsAreNPCsOnly — присутствующие это люди сцены; труп и
// прочие вещи в список не входят.
func TestBuildParticipantsAreNPCsOnly(t *testing.T) {
	g := buildGame(t)
	tv := Build(g, core.TurnResult{}, nil, noRule, noScenario, "")
	var names []string
	for _, w := range tv.Participants {
		names = append(names, w.Name)
	}
	if len(tv.Participants) != 1 || tv.Participants[0].ID != "e_toke" {
		t.Errorf("присутствующие не только NPC: %v", names)
	}
}

// TestBuildNotEndedAtStart — пока игра идёт, развязки нет.
func TestBuildNotEndedAtStart(t *testing.T) {
	g := buildGame(t)
	tv := Build(g, core.TurnResult{}, nil, noRule, noScenario, "")
	if tv.Ended != nil {
		t.Errorf("развязка на старте: %+v", tv.Ended)
	}
}

// TestEndingOf — развязка по состоянию: раскрытое дело важнее висяка, висяк —
// только у остановившегося прогона с текстом.
func TestEndingOf(t *testing.T) {
	if e := endingOf(true, false, ""); e == nil || e.Kind != "solved" {
		t.Errorf("раскрытое дело: %+v", e)
	}
	if e := endingOf(false, true, "холодно"); e == nil || e.Kind != "cold" || e.Text != "холодно" {
		t.Errorf("висяк: %+v", e)
	}
	if e := endingOf(false, false, "холодно"); e != nil {
		t.Errorf("не остановился — не висяк: %+v", e)
	}
	if e := endingOf(false, true, ""); e != nil {
		t.Errorf("висяк без текста молчит: %+v", e)
	}
	if e := endingOf(true, true, "холодно"); e == nil || e.Kind != "solved" {
		t.Errorf("раскрытое важнее висяка: %+v", e)
	}
}
