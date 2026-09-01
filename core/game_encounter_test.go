package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func TestApplyRefusesWhenNotPlayerTurn(t *testing.T) {
	g := &Game{DB: store.NewDB(), Actor: "chr_kay"}
	g.Encounter = &Encounter{Order: []store.EntityID{"e_orc", store.EntityID(g.Actor)}}
	res := g.Apply(Intent{Verb: "attack", Actor: g.Actor, Args: Args{Target: "e_orc"}})
	if !res.Refused {
		t.Fatalf("под боем и не свой ход — обязан быть отказ")
	}
}

func TestApplyCyclesThroughNPCsBackToPlayer(t *testing.T) {
	g := &Game{DB: store.NewDB(), Actor: "chr_kay",
		Scenario: stubNPCScenario{turn: Intent{Verb: "pass"}}}
	g.DB.Entities["e_orc"] = store.Entity{ID: "e_orc", HP: 5, MaxHP: 5}
	g.Encounter = &Encounter{Order: []store.EntityID{store.EntityID(g.Actor), "e_orc"}}
	res := g.Apply(Intent{Verb: "attack", Actor: g.Actor, Args: Args{Target: "e_orc"}})
	_ = res
	if g.Encounter.Current() != store.EntityID(g.Actor) {
		t.Fatalf("после хода игрока и хода NPC — вернуться к игроку: %q", g.Encounter.Current())
	}
}

type stubNPCScenario struct{ turn Intent }

func (s stubNPCScenario) Kind() ScenarioKind                           { return "stub" }
func (s stubNPCScenario) ExtraVerbs() []VerbDef                        { return nil }
func (s stubNPCScenario) Victory(*Game) (bool, string)                 { return false, "" }
func (s stubNPCScenario) Defeat(*Game) (bool, string)                  { return false, "" }
func (s stubNPCScenario) Panel(*Game) Panel                            { return Panel{} }
func (s stubNPCScenario) NPCTurn(*Game, store.EntityID) (Intent, bool) { return s.turn, true }
