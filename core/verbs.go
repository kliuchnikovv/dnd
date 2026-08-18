// Package core держит граф фактов, знание парти, часы, обвинение и таксономию
// цены провала. Ядро не знает ни про конкретную игральную кость, ни про
// атрибуты, ни про то, как считается grit.
package core

type Verb string

type VerbClass string

const (
	ClassNone        VerbClass = "none"
	ClassInvestigate VerbClass = "investigate"
	ClassReason      VerbClass = "reason"
	ClassSocial      VerbClass = "social"
	ClassMove        VerbClass = "move"
	ClassAttack      VerbClass = "attack"
	ClassSupport     VerbClass = "support"
	ClassResource    VerbClass = "resource"
	ClassSkill       VerbClass = "skill"
)

type VerbDef struct {
	Verb  Verb
	Class VerbClass
	// Rolls — требует ли глагол броска. У use_ability и use_item значение
	// перекрывается флагом requires_roll в данных дела.
	Rolls bool
	Hard  bool
}

var Verbs = map[Verb]VerbDef{
	"look":              {"look", ClassNone, false, false},
	"emote":             {"emote", ClassNone, false, false},
	"say":               {"say", ClassNone, false, false},
	"talk_to":           {"talk_to", ClassSocial, false, false},
	"ask_about":         {"ask_about", ClassSocial, false, false},
	"thank":             {"thank", ClassSocial, false, false},
	"threaten_verbally": {"threaten_verbally", ClassSocial, false, false},
	"theorize":          {"theorize", ClassReason, false, false},
	"examine":           {"examine", ClassInvestigate, true, true},
	"search":            {"search", ClassInvestigate, true, true},
	"question":          {"question", ClassInvestigate, true, true},
	"stake_out":         {"stake_out", ClassInvestigate, true, true},
	"tail":              {"tail", ClassInvestigate, true, true},
	"compare":           {"compare", ClassReason, false, true},
	"cross_reference":   {"cross_reference", ClassReason, true, true},
	"strike":            {"strike", ClassAttack, true, true},
	"grapple":           {"grapple", ClassAttack, true, true},
	"move_zone":         {"move_zone", ClassMove, true, true},
	"take_cover":        {"take_cover", ClassMove, true, true},
	"flee":              {"flee", ClassMove, true, true},
	"sneak":             {"sneak", ClassSkill, true, true},
	"pick":              {"pick", ClassSkill, true, true},
	"recall":            {"recall", ClassSkill, true, true},
	"persuade":          {"persuade", ClassSocial, true, true},
	"intimidate":        {"intimidate", ClassSocial, true, true},
	"command":           {"command", ClassSocial, true, true},
	"aid":               {"aid", ClassSupport, true, true},
	"mend":              {"mend", ClassSupport, true, true},
	"use_ability":       {"use_ability", ClassResource, true, true},
	"use_item":          {"use_item", ClassResource, true, true},
}

func LookupVerb(s string) (VerbDef, bool) {
	d, ok := Verbs[Verb(s)]
	return d, ok
}

// AllVerbs возвращает определения в стабильном порядке — для table-driven
// тестов и для вывода help.
func AllVerbs() []VerbDef {
	out := make([]VerbDef, 0, len(Verbs))
	for _, d := range Verbs {
		out = append(out, d)
	}
	sortVerbDefs(out)
	return out
}

func sortVerbDefs(v []VerbDef) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j].Verb < v[j-1].Verb; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
