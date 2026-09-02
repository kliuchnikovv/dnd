package cases

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	// импортируем sub-пакет, чтобы deduction зарегистрировался.
	_ "github.com/kliuchnikovv/dnd/core/scenarios/deduction"
)

func TestLoadDefaultsToDeductionScenario(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scenario == nil {
		t.Fatal("после Load Scenario не должен быть nil")
	}
	if cfg.Scenario.Kind() != core.ScenarioDeduction {
		t.Fatalf("дефолт должен быть deduction, а не %q", cfg.Scenario.Kind())
	}
}

func TestLoadRespectsExplicitAdventure(t *testing.T) {
	_, err := parseWithScenario(t, "adventure")
	if err == nil {
		t.Fatal("без регистрации adventure Load обязан ругнуться")
	}
}

func parseWithScenario(t *testing.T, kind string) (*core.Config, error) {
	t.Helper()
	raw := []byte(`{"id":"c_x","start":"n_x","actor":"chr_x",
      "character":{"id":"chr_x","sheet":{}},
      "locations":[{"id":"n_x","name":"X"}],
      "scenario":"` + kind + `"}`)
	return Parse(raw)
}
