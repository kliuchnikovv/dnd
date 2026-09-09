package cyberpunk

import "github.com/kliuchnikovv/dnd/core"

// check — базовая резолюция RED: STAT + SKILL + 1d10 vs DV.
//
// Крит-успех (натуральная 10): добавляется ЕЩЁ 1d10 в плюс. Стекинга нет
// (крит на крите не тянет третий бросок) — RED явно оговаривает.
// Крит-провал (натуральная 1): вычитается 1d10. Тоже без стекинга.
//
// roll1, roll2 — первый d10 и дополнительный d10 при крите/фамбле. Оба
// передаются параметром, а не читаются из шва — так check тестируется без
// мока кости. Если крита/фамбла нет, roll2 игнорируется.
//
// woundPenalty — суммарный штраф за раны (0/−2/−4/−6 по статусу): его
// вкладывает Resolve, чтобы check оставался чистой функцией без чтения
// сцены и не знал про Seriously/Mortally Wounded.
func check(s Sheet, stat, skill string, dv, woundPenalty, roll1, roll2 int) core.Resolution {
	statBonus := s.Stat(stat)
	skillLvl := s.Skill(skill)

	total := roll1 + statBonus + skillLvl + woundPenalty
	if roll1 == 10 {
		total += roll2 // крит: +1d10, без стекинга
	} else if roll1 == 1 {
		total -= roll2 // фамбл: −1d10, без стекинга
	}

	log := core.RollLog{
		Die:       roll1,
		Threshold: dv,
		Terms: []core.RollTerm{
			{Name: "1d10", Value: roll1},
			{Name: stat, Value: statBonus},
			{Name: "skill:" + skill, Value: skillLvl},
		},
	}
	if roll1 == 10 {
		log.Terms = append(log.Terms, core.RollTerm{Name: "crit:+1d10", Value: roll2})
	} else if roll1 == 1 {
		log.Terms = append(log.Terms, core.RollTerm{Name: "fumble:-1d10", Value: -roll2})
	}
	if woundPenalty != 0 {
		log.Terms = append(log.Terms, core.RollTerm{Name: "wound", Value: woundPenalty})
	}
	log.Total = total

	if total >= dv {
		return core.Resolution{Class: core.OutcomeSuccess, Margin: total - dv, Log: log}
	}
	return core.Resolution{Class: core.OutcomeFail, Margin: dv - total, Log: log}
}
