package view5e

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
)

// gameWithActor — минимальный core.Game с одной сущностью-актёром: view-
// Ruleset читает g.DB.Entities[actor] и g.DB.Conditions[actor], больше ничего
// про игру не знает, поэтому тесту не нужно полное дело.
func gameWithActor(t *testing.T, hp, maxHP int) *core.Game {
	t.Helper()
	actor := store.CharacterID("pc")
	db := store.NewDB()
	db.Entities[store.EntityID(actor)] = store.Entity{
		ID: store.EntityID(actor), HP: hp, MaxHP: maxHP,
	}
	return &core.Game{Actor: actor, DB: db}
}

func meterByKind(ms []view.Meter, kind string) (view.Meter, bool) {
	for _, m := range ms {
		if m.Kind == kind {
			return m, true
		}
	}
	return view.Meter{}, false
}

// TestRulesetRegistered — импорт пакета регистрирует view-фабрику dnd5e.
// Если init не отработает, сервер фолбэкнется на Порог и dnd5e-сессия
// покажет чужие меры — регресс, ради предотвращения которого спека 2 и
// делалась.
func TestRulesetRegistered(t *testing.T) {
	rs, ok := view.LookupRuleset(core.RulesetDND5e)
	if !ok {
		t.Fatal("view.LookupRuleset(dnd5e) не находит фабрики — init не отработал")
	}
	if _, ok := rs.(Ruleset); !ok {
		t.Fatalf("фабрика вернула не dnd5e Ruleset: %T", rs)
	}
}

// TestRulesetHidesHPAtFullHealth — на полном HP мера молчит. Тот же
// surfacing, что у ран Порога, минимализм держится флагом правила.
func TestRulesetHidesHPAtFullHealth(t *testing.T) {
	g := gameWithActor(t, 20, 20)
	hp, ok := meterByKind(Ruleset{}.Meters(g), "hp")
	if !ok {
		t.Fatal("HP-меры нет в наборе")
	}
	if hp.Surface {
		t.Errorf("на полном HP мера всплыла: %+v", hp)
	}
	if hp.Max == nil || *hp.Max != 20 || hp.Value != 20 {
		t.Errorf("не тот HP: %+v", hp)
	}
}

// TestRulesetSurfacesHPWhenWounded — потери HP поднимают меру.
func TestRulesetSurfacesHPWhenWounded(t *testing.T) {
	g := gameWithActor(t, 12, 20)
	hp, _ := meterByKind(Ruleset{}.Meters(g), "hp")
	if !hp.Surface || hp.Value != 12 {
		t.Errorf("ранение не подняло HP-меру: %+v", hp)
	}
}

// TestRulesetSurfacesConditions — каждое активное состояние отдаётся
// отдельной мерой; значение — оставшиеся раунды; порядок стабильный.
func TestRulesetSurfacesConditions(t *testing.T) {
	g := gameWithActor(t, 10, 20)
	actor := store.EntityID(g.Actor)
	g.DB.Conditions[actor] = map[string]int{"bloodied": 0, "poisoned": 3}

	ms := Ruleset{}.Meters(g)
	var conds []view.Meter
	for _, m := range ms {
		if m.Kind == "condition" {
			conds = append(conds, m)
		}
	}
	if len(conds) != 2 {
		t.Fatalf("ожидали 2 состояния, получили %d: %+v", len(conds), conds)
	}
	if conds[0].Label != "bloodied" || conds[1].Label != "poisoned" {
		t.Errorf("порядок состояний нестабильный: %+v", conds)
	}
	if !conds[0].Surface || conds[1].Value != 3 {
		t.Errorf("состояние сложилось неверно: %+v", conds)
	}
}

// TestRulesetSkipsHPWithoutMax — MaxHP=0 (нет HP-модели) не превращается
// в фейковую пару 0/0: такую меру клиент прочитал бы как «мёртв на старте».
func TestRulesetSkipsHPWithoutMax(t *testing.T) {
	g := gameWithActor(t, 0, 0)
	if _, ok := meterByKind(Ruleset{}.Meters(g), "hp"); ok {
		t.Errorf("HP-мера всплыла без MaxHP")
	}
}
