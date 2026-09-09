package cyberpunk

import "github.com/kliuchnikovv/dnd/core"

// attack — атака RED: REF + weapon-skill + 1d10 vs DV. Крит-10/фамбл-1 те же,
// что у check. Пробивной удар вычитает SP цели из урона до HP; если урон
// пробил (dmg > 0), броня цели аблирует на 1 (в мутации-эффекте — фаза A
// упрощение: аблация актёром не отслеживается в SceneView, отражается в
// логе; полный сквозной снос SP цели придёт в фазе B вместе с моделью
// брони на сущностях).
//
// dv — DV атаки (по дальности/цели), targetSP — броня цели (0, если нет),
// weaponDmg — рулон урона оружия, rollD10 — брошенный 1d10, rollExtraD10 —
// доп d10 на крите/фамбле, rollDmgSum — уже сброшенный суммарный урон
// (кости оружия).
func attack(s Sheet, weapon Weapon, dv, targetSP, woundPenalty, rollD10, rollExtraD10, rollDmgSum int) core.Resolution {
	statKey := weapon.Attack
	if statKey == "" {
		statKey = StatREF
	}
	statBonus := s.Stat(statKey)
	skillLvl := s.Skill(weapon.Skill)

	total := rollD10 + statBonus + skillLvl + woundPenalty
	crit := rollD10 == 10
	fumble := rollD10 == 1
	if crit {
		total += rollExtraD10
	} else if fumble {
		total -= rollExtraD10
	}

	log := core.RollLog{
		Die:       rollD10,
		Threshold: dv,
		Terms: []core.RollTerm{
			{Name: "1d10", Value: rollD10},
			{Name: statKey, Value: statBonus},
			{Name: "skill:" + weapon.Skill, Value: skillLvl},
		},
	}
	if crit {
		log.Terms = append(log.Terms, core.RollTerm{Name: "crit:+1d10", Value: rollExtraD10})
	} else if fumble {
		log.Terms = append(log.Terms, core.RollTerm{Name: "fumble:-1d10", Value: -rollExtraD10})
	}
	if woundPenalty != 0 {
		log.Terms = append(log.Terms, core.RollTerm{Name: "wound", Value: woundPenalty})
	}
	log.Total = total

	if total < dv {
		return core.Resolution{Class: core.OutcomeFail, Margin: dv - total, Log: log}
	}

	// Попадание: считаем урон, вычитаем SP. Отрицательный или нулевой урон
	// «до HP» ничего не меняет и не аблирует броню — RED явно оговаривает.
	dmg := rollDmgSum - targetSP
	if dmg < 0 {
		dmg = 0
	}
	class := core.OutcomeSuccess
	if crit {
		class = core.OutcomeCrit
	}
	return core.Resolution{
		Class:  class,
		Margin: total - dv,
		Log:    log,
		Damage: dmg,
	}
}
