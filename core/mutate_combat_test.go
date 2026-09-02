package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func TestMutHPDeltaReducesEntityHP(t *testing.T) {
	g := &Game{DB: store.NewDB()}
	g.DB.Entities["e_orc"] = store.Entity{ID: "e_orc", HP: 10, MaxHP: 10, AC: 13}
	apply(g, Mutation{Kind: MutHPDelta, Target: "e_orc", Amount: -3})
	if g.DB.Entities["e_orc"].HP != 7 {
		t.Fatalf("HP после -3: %d", g.DB.Entities["e_orc"].HP)
	}
}

func TestMutHPDeltaClampsAtZero(t *testing.T) {
	g := &Game{DB: store.NewDB()}
	g.DB.Entities["e_orc"] = store.Entity{ID: "e_orc", HP: 2, MaxHP: 10}
	apply(g, Mutation{Kind: MutHPDelta, Target: "e_orc", Amount: -5})
	if g.DB.Entities["e_orc"].HP != 0 {
		t.Fatalf("HP не клампится на 0: %d", g.DB.Entities["e_orc"].HP)
	}
}

func TestMutHPDeltaHealClampsAtMax(t *testing.T) {
	g := &Game{DB: store.NewDB()}
	g.DB.Entities["e_orc"] = store.Entity{ID: "e_orc", HP: 8, MaxHP: 10}
	apply(g, Mutation{Kind: MutHPDelta, Target: "e_orc", Amount: 5})
	if g.DB.Entities["e_orc"].HP != 10 {
		t.Fatalf("HP не клампится на MaxHP: %d", g.DB.Entities["e_orc"].HP)
	}
}

func TestMutEncounterStartAndEnd(t *testing.T) {
	g := &Game{DB: store.NewDB()}
	apply(g, Mutation{Kind: MutEncounterStart, Order: []store.EntityID{"chr_kay", "e_orc"}})
	if g.Encounter == nil || len(g.Encounter.Order) != 2 {
		t.Fatalf("MutEncounterStart не завёл Encounter: %+v", g.Encounter)
	}
	apply(g, Mutation{Kind: MutAdvanceInitiative})
	if g.Encounter.Current() != "e_orc" {
		t.Fatalf("после Advance не тот текущий: %q", g.Encounter.Current())
	}
	apply(g, Mutation{Kind: MutEncounterEnd})
	if g.Encounter != nil {
		t.Fatalf("MutEncounterEnd не обнулил: %+v", g.Encounter)
	}
}
