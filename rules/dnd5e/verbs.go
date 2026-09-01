package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// Verbs — глаголы D&D, отдаваемые сценарием adventure через ExtraVerbs().
// Общие глаголы (talk_to, examine, ...) не дублируются.
func Verbs() []core.VerbDef {
	return []core.VerbDef{
		{Verb: "attack", Class: core.ClassAttack, Rolls: true, Target: true},
		{Verb: "hide", Class: core.ClassCheck, Rolls: true},
		{Verb: "sneak", Class: core.ClassMove, Rolls: true, Node: true},
		{Verb: "disarm_trap", Class: core.ClassCheck, Rolls: true, Target: true},
		{Verb: "detect_trap", Class: core.ClassCheck, Rolls: true},
		{Verb: "pick_lock", Class: core.ClassCheck, Rolls: true, Target: true},
		{Verb: "flee", Class: core.ClassMove, Rolls: true},
		{Verb: "rest_short", Class: core.ClassRecover},
		{Verb: "rest_long", Class: core.ClassRecover},
	}
}

// DefaultDC — стандартная сложность для проверок D&D.
const DefaultDC = 12
