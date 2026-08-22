package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

type fixedRules struct{ out Outcome }

func (r fixedRules) Resolve(in Intent, _ SceneView, _ Dice) Resolution {
	res := Resolution{Class: r.out, Margin: 0}
	if r.out == OutcomeFail {
		res.Margin = -6
		res.Costs = []CostKind{CostFalseLead}
	}
	return res
}

func turnGame(out Outcome) *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Adjacent: []store.NodeID{"n_forge"}}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Adjacent: []store.NodeID{"n_quay"}}
	db.Entities["e_toke"] = store.Entity{ID: "e_toke", Kind: store.EntityNPC, Node: "n_quay"}
	db.Entities["e_ivar"] = store.Entity{ID: "e_ivar", Kind: store.EntityNPC, Node: "n_forge"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_open"] = store.Fact{ID: "f_open", Key: "open"}
	db.Facts["f_gated"] = store.Fact{ID: "f_gated", Key: "gated"}
	db.Holders["f_open"] = []store.FactHolder{{
		FactID: "f_open", HolderID: "e_toke", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}
	db.Holders["f_gated"] = []store.FactHolder{{
		FactID: "f_gated", HolderID: "e_toke", Mandatory: false,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal",
			Requires: store.RequireAll("f_open")},
	}}
	db.Clocks["c_suspicion"] = &store.Clock{ID: "c_suspicion", Segments: 6, TickPolicy: "on_cost"}
	return NewGame(Config{
		DB: db, Rules: fixedRules{out}, Dice: nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func TestUnknownTopicIsRefusedNotFailed(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	// f_gated парти не знает — темы для вопроса нет.
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_gated"}})
	if !got.Refused {
		t.Fatal("вопрос о неизвестном факте прошёл — защита от угадывания дырявая")
	}
	if got.Res != nil {
		t.Error("отказ дошёл до броска")
	}
	if g.DB.Clocks["c_suspicion"].Filled != 0 {
		t.Error("отказ тикнул часы — ход был потрачен")
	}
}

func TestAbsentTargetIsRefused(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_ivar", Topic: "f_open"}})
	if !got.Refused {
		t.Error("допрошен персонаж из другой локации")
	}
}

func TestWrongVerbForGateIsRefused(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	got := g.Apply(Intent{Verb: "search", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if !got.Refused {
		t.Error("факт выдан глаголом не из gate.verbs")
	}
}

func TestMandatoryFactBypassesTheRoll(t *testing.T) {
	// Кубик всегда проваливает — mandatory-факт обязан выдаться всё равно.
	g := turnGame(OutcomeFail)
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if got.Refused {
		t.Fatalf("mandatory-факт отвергнут: %s", got.Refusal)
	}
	if got.Res != nil {
		t.Error("mandatory-факт прошёл через бросок")
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_open" {
		t.Fatalf("факт не выдан: %v", got.Learned)
	}
	if !g.K.Knows("f_open") {
		t.Error("факт не записан в party_knowledge")
	}
}

func TestGatedFactNeedsItsRequirement(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.K.Learn("f_gated", "e_bern") // тема известна, но требование не выполнено
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_gated"}})
	if !got.Refused {
		t.Error("факт выдан без выполненного requires")
	}
}

func TestFailedRollLearnsNothingAndCosts(t *testing.T) {
	g := turnGame(OutcomeFail)
	g.K.Learn("f_gated", "e_bern")
	g.K.Learn("f_open", "e_bern") // выполняем requires
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_gated"}})
	if got.Refused {
		t.Fatalf("действие отвергнуто вместо провала: %s", got.Refusal)
	}
	for _, l := range got.Learned {
		if l.From == "e_toke" && l.Fact == "f_gated" {
			t.Error("провал выдал факт")
		}
	}
	if !got.FalseLead {
		t.Error("провал investigate не дал ложного следа")
	}
}

func TestSceneViewHidesFactsAndTruth(t *testing.T) {
	// Структурная гарантия: в SceneView нет полей под граф фактов и truth.
	g := turnGame(OutcomeSuccess)
	view := g.SceneView(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if view.Node != "n_quay" {
		t.Errorf("узел в SceneView = %q", view.Node)
	}
	if view.GateThreshold != "normal" {
		t.Errorf("сложность gate не доехала до правил: %q", view.GateThreshold)
	}
}

// Проп — законная цель свободной пробы. Если бы обращение к пропу отвечало
// «такой сущности в деле нет», отказ сам сообщал бы игроку, какие цели
// настоящие, — и весь камуфляж не стоил бы ничего.
func TestPropIsAValidTargetAndGivesProse(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Props["n_quay"] = []store.SceneProp{{
		ID: "p_crates", Node: "n_quay", Name: "Ящики", Tags: []string{"cover"},
	}}

	res := g.Apply(Intent{Verb: "look", Args: Args{Target: "p_crates"}})
	if res.Refused {
		t.Fatalf("проп отвергнут как цель: %s", res.Refusal)
	}
	if res.FlavourKey != "prop.p_crates" {
		t.Errorf("ключ прозы пропа %q, ожидался prop.p_crates", res.FlavourKey)
	}
}

// Жёсткая проба по пропу ведёт себя ровно как по сущности без фактов: бросок
// есть, факта нет. Разница в поведении выдала бы граф не хуже разметки.
func TestHardVerbOnPropRollsAndFindsNothing(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Props["n_quay"] = []store.SceneProp{{
		ID: "p_crates", Node: "n_quay", Name: "Ящики", Tags: []string{"cover"},
	}}

	res := g.Apply(Intent{Verb: "examine", Args: Args{Target: "p_crates"}})
	if res.Refused {
		t.Fatalf("осмотр пропа отвергнут: %s", res.Refusal)
	}
	if res.Res == nil {
		t.Error("жёсткая проба по пропу прошла без броска")
	}
	if len(res.Learned) != 0 {
		t.Errorf("проп выдал факт: %v", res.Learned)
	}
}

// Инструмент берётся ходом. Плата за отмену штрафа среды — потраченное
// действие, и она берётся именно здесь: нарратор может поставить фонарь в
// сцену, но взять его в руки может только игрок.
func TestUsingAToolPropMakesItActiveInTheScene(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Props["n_quay"] = []store.SceneProp{{
		ID: "p_lantern", Node: "n_quay", Name: "Фонарь", Kind: store.ToolKind,
	}}

	if tools := g.SceneView(Intent{Verb: "examine"}).Tools; len(tools) != 0 {
		t.Fatalf("инструмент активен без хода: %v", tools)
	}
	if res := g.Apply(Intent{Verb: "use_item", Args: Args{Item: "p_lantern"}}); res.Refused {
		t.Fatalf("взять фонарь не вышло: %s", res.Refusal)
	}
	if tools := g.SceneView(Intent{Verb: "examine"}).Tools; len(tools) != 1 || tools[0] != "p_lantern" {
		t.Errorf("инструмент не попал в сцену: %v", tools)
	}
}

// Инструмент остаётся в узле, где его взяли: фонарь со склада не светит в
// конторе гильдии.
func TestToolDoesNotTravelBetweenNodes(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Props["n_quay"] = []store.SceneProp{{
		ID: "p_lantern", Node: "n_quay", Name: "Фонарь", Kind: store.ToolKind,
	}}
	g.Apply(Intent{Verb: "use_item", Args: Args{Item: "p_lantern"}})
	g.Node = "n_forge"
	if tools := g.SceneView(Intent{Verb: "examine"}).Tools; len(tools) != 0 {
		t.Errorf("инструмент уехал в другой узел: %v", tools)
	}
}
