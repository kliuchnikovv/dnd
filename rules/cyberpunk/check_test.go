package cyberpunk

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

func TestCheckSuccess(t *testing.T) {
	s := Sheet{REF: 5, Skills: map[string]int{"handgun": 4}}
	// 1d10=7, REF=5, skill=4, total=16, DV=15 → success с margin 1.
	res := check(s, StatREF, "handgun", DVDifficult, 0, 7, 0)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("class %v, want success", res.Class)
	}
	if res.Margin != 1 {
		t.Fatalf("margin %d, want 1", res.Margin)
	}
}

func TestCheckFail(t *testing.T) {
	s := Sheet{REF: 3, Skills: map[string]int{"handgun": 2}}
	// 1d10=2, REF=3, skill=2, total=7, DV=15 → fail с margin 8.
	res := check(s, StatREF, "handgun", DVDifficult, 0, 2, 0)
	if res.Class != core.OutcomeFail {
		t.Fatalf("class %v, want fail", res.Class)
	}
	if res.Margin != 8 {
		t.Fatalf("margin %d, want 8", res.Margin)
	}
}

func TestCheckCritAddsExtraD10(t *testing.T) {
	s := Sheet{REF: 2, Skills: map[string]int{"handgun": 2}}
	// нат-10 плюс roll2=5, всего 10+5+2+2 = 19 против DV=15.
	res := check(s, StatREF, "handgun", DVDifficult, 0, 10, 5)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("class %v", res.Class)
	}
	if res.Margin != 4 {
		t.Fatalf("margin %d, want 4 (крит подкинул +5)", res.Margin)
	}
}

func TestCheckFumbleSubtractsExtraD10(t *testing.T) {
	s := Sheet{REF: 8, Skills: map[string]int{"handgun": 8}}
	// нат-1 плюс roll2=5, total = 1+8+8−5 = 12 против DV=15 → fail.
	res := check(s, StatREF, "handgun", DVDifficult, 0, 1, 5)
	if res.Class != core.OutcomeFail {
		t.Fatalf("class %v, want fail (фамбл), got margin=%d", res.Class, res.Margin)
	}
	if res.Margin != 3 {
		t.Fatalf("margin %d, want 3", res.Margin)
	}
}

func TestCheckCritNoStacking(t *testing.T) {
	// Дополнительный d10 на крите НЕ вызывает второго доп-броска, даже если
	// roll2 тоже 10. Тест доказывает это тем, что итог равен ровно
	// roll1+roll2+stat+skill, без третьего слагаемого-роля.
	s := Sheet{REF: 0, Skills: map[string]int{"handgun": 0}}
	res := check(s, StatREF, "handgun", DVEveryday, 0, 10, 10)
	if res.Margin != 10+10-DVEveryday {
		t.Fatalf("margin %d, want %d — крит не должен стекаться", res.Margin, 10+10-DVEveryday)
	}
}

func TestCheckWoundPenaltyApplied(t *testing.T) {
	s := Sheet{REF: 5, Skills: map[string]int{"handgun": 4}}
	// Без раны total 7+5+4=16 → success. С Seriously Wounded (−2) 14 → fail.
	res := check(s, StatREF, "handgun", DVDifficult, -2, 7, 0)
	if res.Class != core.OutcomeFail {
		t.Fatalf("рана должна утопить, got %v (margin=%d)", res.Class, res.Margin)
	}
}

func TestCheckWithoutSkillStillRolls(t *testing.T) {
	// Незнакомый навык (пустая карта) — 0, но проверка не отказывается: голый
	// d10+STAT vs DV. Это свойство RED: попытаться можно всегда.
	s := Sheet{REF: 5}
	res := check(s, StatREF, "handgun", DVEveryday, 0, 8, 0)
	// total = 8+5+0 = 13 = DV → success.
	if res.Class != core.OutcomeSuccess || res.Margin != 0 {
		t.Fatalf("class=%v margin=%d, want success/0", res.Class, res.Margin)
	}
}
