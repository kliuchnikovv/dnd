package dnd5e

import (
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

func TestAttackHitDealsDamage(t *testing.T) {
	s := Sheet{Str: 16, Prof: 2} // mod str = 3
	dmg := DiceSpec{N: 1, Sides: 8, AbilityMod: "str"}
	// roll=12: 12+3+2=17 против AC 15 — попадание.
	res := attack(s, "str", 15, 12, dmg, func(sp DiceSpec, crit bool) int {
		if crit {
			t.Fatal("не крит — rollDmg не должен получить crit=true")
		}
		return sp.Mod + s.Mod(sp.AbilityMod) + 5 // фиксированная «кость»=5
	})
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("попадание ожидалось, получен %v", res.Class)
	}
	if res.Damage != 8 { // 5 + str(3)
		t.Fatalf("урон: ждали 8, получили %d", res.Damage)
	}
	if res.Margin != 2 {
		t.Fatalf("margin: ждали 2, получили %d", res.Margin)
	}
}

func TestAttackMissDealsNoDamage(t *testing.T) {
	s := Sheet{Str: 16, Prof: 2}
	dmg := DiceSpec{N: 1, Sides: 8, AbilityMod: "str"}
	called := false
	// roll=2: 2+3+2=7 против AC 15 — промах.
	res := attack(s, "str", 15, 2, dmg, func(sp DiceSpec, crit bool) int {
		called = true
		return 99
	})
	if res.Class != core.OutcomeFail {
		t.Fatalf("промах ожидался, получен %v", res.Class)
	}
	if res.Damage != 0 {
		t.Fatalf("урон при промахе должен быть 0, получили %d", res.Damage)
	}
	if called {
		t.Fatal("rollDmg не должен вызываться при промахе")
	}
}

func TestAttackNat20AlwaysHitsAndCrits(t *testing.T) {
	s := Sheet{Str: 8, Prof: 2} // mod str = -1, итог заведомо меньше AC
	dmg := DiceSpec{N: 1, Sides: 8, AbilityMod: "str"}
	gotCrit := false
	res := attack(s, "str", 30, 20, dmg, func(sp DiceSpec, crit bool) int {
		gotCrit = crit
		return 10
	})
	if !gotCrit {
		t.Fatal("нат-20 обязан передать crit=true в rollDmg")
	}
	if res.Class != core.OutcomeCrit {
		t.Fatalf("класс крита ожидался, получен %v", res.Class)
	}
	if res.Damage != 10 {
		t.Fatalf("урон крита: ждали 10, получили %d", res.Damage)
	}
}

// TestSystemResolveAttackDoublesDiceOnCrit — интеграционный тест через
// System.Resolve: проверяет, что resolveAttack действительно удваивает N
// костей урона на крите (а не только флаг crit).
func TestSystemResolveAttackDoublesDiceOnCrit(t *testing.T) {
	s := Sheet{Str: 14, Prof: 2, Weapons: []Weapon{{
		Name: "меч", Reach: "melee", Attack: "str", Damage: "1d8+str",
	}}}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	view := core.SceneView{Sheet: raw, TargetAC: 10}
	sys := New()
	// FixedDice отдаёт 20 на бросок атаки и потом одно и то же значение на
	// каждую кость урона: d.Roll(n, sides) в dice.FixedDice.Roll игнорирует
	// n и sides, всегда возвращая следующее фиксированное значение D20().
	d := dice.Fixed(20, 6)
	res := sys.Resolve(core.Intent{Verb: "strike"}, view, d)
	if res.Class != core.OutcomeCrit {
		t.Fatalf("крит ожидался, получен %v", res.Class)
	}
}
