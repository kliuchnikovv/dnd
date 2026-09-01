package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// System реализует core.RuleSystem — лёгкое подмножество 5e: check, attack,
// save. Никакого состояния между вызовами.
type System struct{}

func New() *System { return &System{} }

func (s *System) Resolve(in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
	def := core.Verbs[in.Verb]

	// Глагол без броска просто удаётся — как и в threshold.System.Resolve:
	// это свойство глагола, а не системы правил.
	if !def.Rolls || in.Verb == "move_zone" {
		return core.Resolution{Class: core.OutcomeSuccess}
	}

	sheet, _ := ParseSheet(view.Sheet)
	switch def.Class {
	case core.ClassAttack:
		return resolveAttack(sheet, in, view, d)
	case core.ClassSave:
		return resolveSave(sheet, in, view, d)
	default:
		return resolveCheck(sheet, in, view, d)
	}
}

// rollD20 бросает 1d20 через шов core.Dice: Roll(n, sides) с n=1.
func rollD20(d core.Dice) int { return d.Roll(1, 20) }

func resolveCheck(s Sheet, in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
	skill := SkillOfVerb(in.Verb)
	dc := DCOfIntent(in, view)
	return check(s, skill, dc, rollD20(d))
}

func resolveAttack(s Sheet, in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
	if len(s.Weapons) == 0 {
		// Нет оружия в листе — атака промахивается сама собой, без броска.
		// Выбор оружия из нескольких — задача авторства кейса (MVP: первое).
		return core.Resolution{Class: core.OutcomeFail}
	}
	w := s.Weapons[0]
	dmgSpec, _ := ParseDice(w.Damage)
	ac := view.TargetAC
	return attack(s, w.Attack, ac, rollD20(d), dmgSpec, func(sp DiceSpec, crit bool) int {
		n := sp.N
		if crit {
			n *= 2
		}
		total := sp.Mod + s.Mod(sp.AbilityMod)
		total += d.Roll(n, sp.Sides)
		return total
	})
}

func resolveSave(s Sheet, in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
	return save(s, view.SaveAbility, view.DC, rollD20(d))
}

// verbSkill — какой skill проверяет глагол по умолчанию, если интент не
// называет его явно. Таблица закрыта и намеренно небольшая: расширять её —
// задача данных дела (case.json), а не резолвера.
var verbSkill = map[core.Verb]string{
	"sneak":        "stealth",
	"hide":         "stealth",
	"pick":         "sleight_of_hand",
	"pick_lock":    "thieves_tools",
	"disarm_trap":  "thieves_tools",
	"detect_trap":  "perception",
	"flee":         "acrobatics",
	"recall":       "history",
	"persuade":     "persuasion",
	"intimidate":   "intimidation",
	"command":      "intimidation",
	"search":       "investigation",
	"examine":      "investigation",
	"question":     "insight",
}

// SkillOfVerb возвращает skill, связанный с глаголом по умолчанию.
// Для неизвестного глагола — пустую строку: check тогда бросает голый d20
// без ability-бонуса и без prof.
func SkillOfVerb(v core.Verb) string {
	return verbSkill[v]
}

// DCOfIntent — сложность проверки для данного интента. Явно проставленный
// SceneView.DC приоритетнее: это то, что авторство кейса уже решило.
// Иначе — заглушка CheckDC (данные "checks:" ещё не подключены, задача
// авторства кейса вне этой задачи).
func DCOfIntent(in core.Intent, view core.SceneView) int {
	if view.DC != 0 {
		return view.DC
	}
	return view.CheckDC(string(in.Verb), string(in.Args.Target))
}
