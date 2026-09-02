package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// attack — d20 + ability + prof против AC цели. Натуральная 20 — крит:
// попадание засчитывается независимо от AC, а урон удваивается по костям
// (rollDmg сам решает, что значит crit=true для конкретного DiceSpec).
//
// weaponAbility — характеристика атаки оружия ("str" для melee, "dex" для
// ranged/finesse); damage — уже разобранный DiceSpec урона оружия; rollDmg —
// внешняя функция суммирования урона по костям, чтобы attack оставался
// чистым и тестируемым без обращения к живой кости.
func attack(s Sheet, weaponAbility string, ac int, roll int,
	damage DiceSpec, rollDmg func(spec DiceSpec, crit bool) int) core.Resolution {

	total := roll + s.Mod(weaponAbility) + s.Prof
	crit := roll == 20
	if !crit && total < ac {
		return core.Resolution{Class: core.OutcomeFail, Margin: ac - total}
	}
	dmg := rollDmg(damage, crit)
	class := core.OutcomeSuccess
	if crit {
		class = core.OutcomeCrit
	}
	return core.Resolution{Class: class, Margin: total - ac, Damage: dmg}
}
