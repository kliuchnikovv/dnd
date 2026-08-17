package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

type nilRules struct{}

func (nilRules) Resolve(Intent, SceneView, Dice) Resolution { return Resolution{} }

type nilDice struct{}

func (nilDice) D20() int          { return 10 }
func (nilDice) Roll(int, int) int { return 3 }

func testGame() *Game {
	db := store.NewDB()
	db.Entities["e_toke"] = store.Entity{ID: "e_toke", Kind: store.EntityNPC, Node: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3, Harm: 0}
	db.Clocks["c_suspicion"] = &store.Clock{
		ID: "c_suspicion", Segments: 2, TickPolicy: "on_cost",
		OnFill: store.Consequence{FlavourKey: "clock.filled", HostileTo: []store.EntityID{"e_toke"}},
	}
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	return NewGame(Config{
		DB:      db,
		Rules:   nilRules{},
		Dice:    nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{"clock.filled": "Прилив забрал следы."},
		Start:   "n_quay",
		Actor:   "pc",
	})
}

func TestMutationsAreAppliedByName(t *testing.T) {
	g := testGame()
	g.applyMutations([]Mutation{
		{Kind: MutResource, Target: "grit", Delta: -1},
		{Kind: MutHarm, Target: "pc", Delta: 1},
		{Kind: MutDisposition, Target: "e_toke", Delta: -1},
	})
	if got := g.DB.Characters["pc"].Grit; got != 2 {
		t.Errorf("grit = %d, ожидалось 2", got)
	}
	if got := g.DB.Characters["pc"].Harm; got != 1 {
		t.Errorf("harm = %d, ожидалось 1", got)
	}
	if got := g.Disposition["e_toke"]; got != -1 {
		t.Errorf("disposition = %d, ожидалось -1", got)
	}
}

func TestUnknownResourceIsIgnoredNotPanicked(t *testing.T) {
	// Имена ресурсов даёт система правил. Ядро их не интерпретирует и не
	// обязано знать: незнакомое имя не должно ронять прогон.
	g := testGame()
	g.applyMutations([]Mutation{{Kind: MutResource, Target: "slot_3", Delta: -1}})
	if g.DB.Characters["pc"].Grit != 3 {
		t.Error("незнакомый ресурс задел grit")
	}
}

func TestEveryCostKindExecutes(t *testing.T) {
	in := Intent{Verb: "question", Args: Args{Target: "e_toke"}}
	for _, kind := range AllCostKinds() {
		g := testGame()
		before := snapshot(g)
		g.executeCosts([]CostKind{kind}, in)
		if kind == CostHalfEffect || kind == CostResourceSpent || kind == CostFalseLead {
			// Эти три меняют не состояние мира, а исход хода: их эффект
			// проверяется в TurnResult (Task 13).
			continue
		}
		if snapshot(g) == before {
			t.Errorf("цена %q не изменила состояние — не исполнена", kind)
		}
	}
}

func TestTickClockCostFiresConsequence(t *testing.T) {
	g := testGame()
	in := Intent{Verb: "question", Args: Args{Target: "e_toke"}}
	if got := g.executeCosts([]CostKind{CostTickClock}, in); len(got) != 0 {
		t.Fatalf("часы сработали на первом тике из двух: %v", got)
	}
	got := g.executeCosts([]CostKind{CostTickClock}, in)
	if len(got) != 1 || got[0].FlavourKey != "clock.filled" {
		t.Fatalf("последствие не сработало: %v", got)
	}
	g.applyConsequences(got)
	if g.Disposition["e_toke"] >= 0 {
		t.Error("HostileTo не понизил расположение")
	}
}

func TestFlavourFallsBackToKey(t *testing.T) {
	g := testGame()
	if got := g.Flavour("clock.filled"); got != "Прилив забрал следы." {
		t.Errorf("текст не найден: %q", got)
	}
	// Отсутствующий ключ виден сразу, а не молча превращается в пустую строку.
	if got := g.Flavour("нет.такого.ключа"); got != "[нет.такого.ключа]" {
		t.Errorf("отсутствующий ключ = %q, ожидалось [нет.такого.ключа]", got)
	}
}

func snapshot(g *Game) [4]int {
	c := g.DB.Characters["pc"]
	return [4]int{c.Grit, c.Harm, g.Disposition["e_toke"] + g.Debts["e_toke"],
		g.DB.Clocks["c_suspicion"].Filled + boolInt(g.Detected)}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
