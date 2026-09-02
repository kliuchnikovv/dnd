package lighthouse_test

import (
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	_ "github.com/kliuchnikovv/dnd/core/scenarios/adventure"
	"github.com/kliuchnikovv/dnd/rules/dnd5e"
	"github.com/kliuchnikovv/dnd/store"
)

// TestLighthouseLoads — смок-тест регрессии: дело загружается без ошибок,
// сценарий и система правил — D&D-приключение, а не расследование по умолчанию.
//
// cases.Parse не умеет разбирать поле "rules" (в схеме его вовсе нет) —
// единственную систему правил, которую понимает загрузчик, подставляет
// вызывающий сам после Load (см. core/scenarios/adventure/mini_e2e_test.go).
func TestLighthouseLoads(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatalf("дело не проходит валидацию:\n%v", err)
	}
	if cfg.Scenario == nil {
		t.Fatal("Scenario не должен быть nil")
	}
	if cfg.Scenario.Kind() != core.ScenarioAdventure {
		t.Fatalf("сценарий: %v, ожидался adventure", cfg.Scenario.Kind())
	}

	cfg.Rules = dnd5e.New()
	if cfg.Rules == nil {
		t.Fatal("Rules не должен быть nil после подстановки dnd5e")
	}

	if _, ok := cfg.DB.Characters[store.CharacterID(cfg.Actor)]; !ok {
		t.Fatalf("персонаж-актёр %q не найден", cfg.Actor)
	}

	for _, n := range []store.NodeID{
		"n_shore", "n_path", "n_ruins", "n_gate", "n_stairs", "n_lantern",
	} {
		if _, ok := cfg.DB.Locations[n]; !ok {
			t.Errorf("узел %s отсутствует", n)
		}
	}

	for _, e := range []store.EntityID{
		"chr_kay", "e_fisher", "e_talan", "e_thief", "e_harpy1", "e_harpy2", "e_harpy3",
	} {
		if _, ok := cfg.DB.Entities[e]; !ok {
			t.Errorf("сущность %s отсутствует", e)
		}
	}
}
