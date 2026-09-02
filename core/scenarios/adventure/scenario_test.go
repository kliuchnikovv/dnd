package adventure_test

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	_ "github.com/kliuchnikovv/dnd/core/scenarios/adventure"
)

func TestAdventureRegistered(t *testing.T) {
	sc, ok := core.LookupScenario(core.ScenarioAdventure)
	if !ok {
		t.Fatal("adventure не зарегистрирован")
	}
	if sc.Kind() != core.ScenarioAdventure {
		t.Fatal("kind разъехался")
	}
}
