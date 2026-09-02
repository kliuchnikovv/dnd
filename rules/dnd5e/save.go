package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// save — d20 + ability(+ prof, если спас свой) против DC. В отличие от
// check, ability здесь не выводится из skill — она приходит прямо от
// эффекта, вызвавшего спасбросок (яд бьёт по con, заклинание — по wis и т.д.).
func save(s Sheet, ability string, dc int, roll int) core.Resolution {
	total := roll + s.Mod(ability)
	if s.SaveProficient(ability) {
		total += s.Prof
	}
	if total >= dc {
		return core.Resolution{Class: core.OutcomeSuccess, Margin: total - dc}
	}
	return core.Resolution{Class: core.OutcomeFail, Margin: dc - total}
}
