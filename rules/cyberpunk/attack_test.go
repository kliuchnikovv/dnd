package cyberpunk

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

func TestAttackHitCarriesDamageMinusSP(t *testing.T) {
	s := Sheet{REF: 6, Skills: map[string]int{"handgun": 4}}
	w := Weapon{Name: "Medium Pistol", Skill: "handgun", Damage: "2d6"}
	// 1d10=7 + REF6 + skill4 = 17 vs DV=15 → hit. Урон dmgSum=8, SP цели=3
	// → в HP уходит 5.
	res := attack(s, w, DVDifficult, 3, 0, 7, 0, 8)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("class %v", res.Class)
	}
	if res.Damage != 5 {
		t.Fatalf("damage %d, want 5 (8−SP3)", res.Damage)
	}
}

func TestAttackMissesBelowDV(t *testing.T) {
	s := Sheet{REF: 2, Skills: map[string]int{"handgun": 1}}
	w := Weapon{Name: "Medium Pistol", Skill: "handgun", Damage: "2d6"}
	// 3+2+1 = 6 vs DV=15 → miss.
	res := attack(s, w, DVDifficult, 0, 0, 3, 0, 10)
	if res.Class != core.OutcomeFail {
		t.Fatalf("class %v, want fail", res.Class)
	}
	if res.Damage != 0 {
		t.Fatalf("damage %d, want 0", res.Damage)
	}
}

func TestAttackDamageBlockedByArmor(t *testing.T) {
	s := Sheet{REF: 6, Skills: map[string]int{"handgun": 4}}
	w := Weapon{Name: "Medium Pistol", Skill: "handgun", Damage: "2d6"}
	// Попал, но урон 4, SP 11 → в HP 0. RED: если не пробило — не меняется.
	res := attack(s, w, DVDifficult, 11, 0, 7, 0, 4)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("class %v", res.Class)
	}
	if res.Damage != 0 {
		t.Fatalf("damage %d, want 0 (броня 11 vs 4)", res.Damage)
	}
}

func TestAttackCritClassAndBonus(t *testing.T) {
	s := Sheet{REF: 4, Skills: map[string]int{"handgun": 2}}
	w := Weapon{Name: "Medium Pistol", Skill: "handgun", Damage: "2d6"}
	// нат-10 + extra=5 → total 10+5+4+2=21. DV=15. Крит.
	res := attack(s, w, DVDifficult, 0, 0, 10, 5, 7)
	if res.Class != core.OutcomeCrit {
		t.Fatalf("class %v, want crit", res.Class)
	}
	if res.Damage != 7 {
		t.Fatalf("damage %d, want 7", res.Damage)
	}
}
