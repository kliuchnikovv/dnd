package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func TestScenarioRegistryReturnsRegistered(t *testing.T) {
	stub := stubScenario{kind: "test"}
	RegisterScenario(stub.kind, func() Scenario { return stub })
	got, ok := LookupScenario("test")
	if !ok {
		t.Fatalf("registry не вернул зарегистрированный сценарий")
	}
	if got.Kind() != "test" {
		t.Fatalf("kind разошёлся: %q", got.Kind())
	}
}

func TestScenarioRegistryUnknownIsFalse(t *testing.T) {
	if _, ok := LookupScenario("no-such"); ok {
		t.Fatal("registry соврал про незнакомый сценарий")
	}
}

type stubScenario struct{ kind ScenarioKind }

func (s stubScenario) Kind() ScenarioKind { return s.kind }
func (s stubScenario) ExtraVerbs() []VerbDef { return nil }
func (s stubScenario) Victory(*Game) (bool, string) { return false, "" }
func (s stubScenario) Defeat(*Game) (bool, string) { return false, "" }
func (s stubScenario) Panel(*Game) Panel { return Panel{} }
func (s stubScenario) NPCTurn(*Game, store.EntityID) (Intent, bool) { return Intent{}, false }
