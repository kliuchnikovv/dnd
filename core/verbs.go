// Package core держит граф фактов, знание парти, часы, обвинение и таксономию
// цены провала. Ядро не знает ни про конкретную игральную кость, ни про
// атрибуты, ни про то, как считается grit.
package core

import "sort"

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
	// ClassSave — спасбросок: атрибут защищающегося против DC, а не проверка
	// навыка атакующего. Пока ни один глагол реестра её не носит — класс
	// назначает система правил (dnd5e) сама себе на резолве save-эффектов;
	// данные дела получат явные save-глаголы позже.
	ClassSave VerbClass = "save"
)

type VerbDef struct {
	Verb  Verb
	Class VerbClass
	// Rolls — требует ли глагол броска. У use_ability и use_item значение
	// перекрывается флагом requires_roll в данных дела.
	Rolls bool
	Hard  bool
	// Spends — глагол тратит игровое время сам по себе, удачно или нет.
	// Просидеть вечер в тени бочек стоит вечера. Это не цена провала, а
	// свойство действия, поэтому живёт в ядре и от системы правил не зависит.
	Spends bool
}

var Verbs = map[Verb]VerbDef{
	"look":    {"look", ClassNone, false, false, false},
	"emote":   {"emote", ClassNone, false, false, false},
	"say":     {"say", ClassNone, false, false, false},
	"talk_to": {"talk_to", ClassSocial, false, false, false},
	// present — предъявить предмет. Социальный, потому что цель — человек: к
	// нему применимы и сдвиг расположения, и социальные гейты. Без броска:
	// годная бумага — это власть, а не проба навыка, и блеф негодной бумагой
	// это другой глагол, которого пока нет.
	"present":           {"present", ClassSocial, false, false, false},
	"ask_about":         {"ask_about", ClassSocial, false, false, false},
	"thank":             {"thank", ClassSocial, false, false, false},
	"threaten_verbally": {"threaten_verbally", ClassSocial, false, false, false},
	"theorize":          {"theorize", ClassReason, false, false, false},
	"examine":           {"examine", ClassInvestigate, true, true, false},
	"search":            {"search", ClassInvestigate, true, true, false},
	"question":          {"question", ClassInvestigate, true, true, false},
	"stake_out":         {"stake_out", ClassInvestigate, true, true, true},
	"tail":              {"tail", ClassInvestigate, true, true, true},
	"compare":           {"compare", ClassReason, false, true, false},
	"cross_reference":   {"cross_reference", ClassReason, true, true, false},
	"strike":            {"strike", ClassAttack, true, true, false},
	"grapple":           {"grapple", ClassAttack, true, true, false},
	"move_zone":         {"move_zone", ClassMove, true, true, false},
	"take_cover":        {"take_cover", ClassMove, true, true, false},
	"flee":              {"flee", ClassMove, true, true, false},
	"sneak":             {"sneak", ClassSkill, true, true, false},
	"pick":              {"pick", ClassSkill, true, true, false},
	"recall":            {"recall", ClassSkill, true, true, false},
	"persuade":          {"persuade", ClassSocial, true, true, false},
	"intimidate":        {"intimidate", ClassSocial, true, true, false},
	"command":           {"command", ClassSocial, true, true, false},
	"aid":               {"aid", ClassSupport, true, true, false},
	"mend":              {"mend", ClassSupport, true, true, false},
	"use_ability":       {"use_ability", ClassResource, true, true, false},
	"use_item":          {"use_item", ClassResource, true, true, false},
}

// classes — закрытый реестр классов. Существует потому, что класс приходит
// СНАРУЖИ: его предлагает парсер по свободному тексту игрока, и принимать его
// на слово нельзя. Выдуманное значение здесь не «неизвестный класс», а
// отсутствие подсказки.
var classes = map[VerbClass]bool{
	ClassNone: true, ClassInvestigate: true, ClassReason: true,
	ClassSocial: true, ClassMove: true, ClassAttack: true,
	ClassSupport: true, ClassResource: true, ClassSkill: true,
	ClassSave: true,
}

// LookupClass проверяет недоверенную подсказку класса по реестру ядра.
//
// «none» отвергается наравне с выдуманным: это класс глаголов, которые ничего
// не делают (look, emote, say), и как подсказка о форме он неотличим от её
// отсутствия. Отдельное значение «форма есть, но никакая» только добавило бы
// вызывающим ветку, ведущую туда же.
func LookupClass(s string) (VerbClass, bool) {
	c := VerbClass(s)
	if s == "" || c == ClassNone || !classes[c] {
		return "", false
	}
	return c, true
}

// AllClasses — классы реестра в стабильном порядке. Нужен слою над доменом:
// перечисление в схеме парсера обязано браться отсюда, иначе появится второе
// место правды о том, какие классы бывают, и разойдётся оно молча.
func AllClasses() []VerbClass {
	out := make([]VerbClass, 0, len(classes))
	for c := range classes {
		if c != ClassNone {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
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
