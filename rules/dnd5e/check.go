package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// check — d20 + ability(+ prof, если проверка своя) против DC. Ability
// определяется таблицей SkillAbility по skill; пустой skill даёт "" —
// тогда ability не найдена, Mod("") трактуется как модификатор 10 (0), а
// Proficient("") всегда ложь, то есть чистый бросок без бонусов.
//
// roll — уже брошенный d20 (1..20), передаётся параметром, а не читается
// из кости здесь: это делает check чистой функцией, тестируемой без мока
// генератора случайных чисел.
func check(s Sheet, skill string, dc int, roll int) core.Resolution {
	ability := SkillAbility(skill)
	total := roll + s.Mod(ability)
	if s.Proficient(skill) {
		total += s.Prof
	}
	if total >= dc {
		return core.Resolution{Class: core.OutcomeSuccess, Margin: total - dc}
	}
	return core.Resolution{Class: core.OutcomeFail, Margin: dc - total}
}
