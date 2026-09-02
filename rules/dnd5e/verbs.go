package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// Verbs — глаголы D&D, отдаваемые сценарием adventure через ExtraVerbs().
// Общие глаголы (talk_to, examine, sneak, flee, ...) не дублируются.
func Verbs() []core.VerbDef {
	return []core.VerbDef{
		{Verb: "attack", Class: core.ClassAttack, Rolls: true, Hard: false, Spends: false},
		{Verb: "hide", Class: core.ClassCheck, Rolls: true, Hard: false, Spends: false},
		{Verb: "disarm_trap", Class: core.ClassCheck, Rolls: true, Hard: false, Spends: false},
		{Verb: "detect_trap", Class: core.ClassCheck, Rolls: true, Hard: false, Spends: false},
		{Verb: "pick_lock", Class: core.ClassCheck, Rolls: true, Hard: false, Spends: false},
		{Verb: "rest_short", Class: core.ClassRecover, Rolls: false, Hard: false, Spends: false},
		{Verb: "rest_long", Class: core.ClassRecover, Rolls: false, Hard: false, Spends: false},
	}
}

// DefaultDC — стандартная сложность для проверок D&D.
const DefaultDC = 12
