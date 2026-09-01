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
	ClassSave     VerbClass = "save"
	ClassCheck    VerbClass = "check"
	ClassRecover  VerbClass = "recover"
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
	// Target — глагол требует цель для действия.
	Target bool
	// Node — глагол привязан к узлу сцены.
	Node bool
}

var Verbs = map[Verb]VerbDef{
	"look":    {Verb: "look", Class: ClassNone, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	"emote":   {Verb: "emote", Class: ClassNone, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	"say":     {Verb: "say", Class: ClassNone, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	"talk_to": {Verb: "talk_to", Class: ClassSocial, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	// present — предъявить предмет. Социальный, потому что цель — человек: к
	// нему применимы и сдвиг расположения, и социальные гейты. Без броска:
	// годная бумага — это власть, а не проба навыка, и блеф негодной бумагой
	// это другой глагол, которого пока нет.
	"present":           {Verb: "present", Class: ClassSocial, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	"ask_about":         {Verb: "ask_about", Class: ClassSocial, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	"thank":             {Verb: "thank", Class: ClassSocial, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	"threaten_verbally": {Verb: "threaten_verbally", Class: ClassSocial, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	"theorize":          {Verb: "theorize", Class: ClassReason, Rolls: false, Hard: false, Spends: false, Target: false, Node: false},
	"examine":           {Verb: "examine", Class: ClassInvestigate, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"search":            {Verb: "search", Class: ClassInvestigate, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"question":          {Verb: "question", Class: ClassInvestigate, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"stake_out":         {Verb: "stake_out", Class: ClassInvestigate, Rolls: true, Hard: true, Spends: true, Target: false, Node: true},
	"tail":              {Verb: "tail", Class: ClassInvestigate, Rolls: true, Hard: true, Spends: true, Target: false, Node: true},
	"compare":           {Verb: "compare", Class: ClassReason, Rolls: false, Hard: true, Spends: false, Target: false, Node: false},
	"cross_reference":   {Verb: "cross_reference", Class: ClassReason, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"strike":            {Verb: "strike", Class: ClassAttack, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"grapple":           {Verb: "grapple", Class: ClassAttack, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"move_zone":         {Verb: "move_zone", Class: ClassMove, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"take_cover":        {Verb: "take_cover", Class: ClassMove, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"flee":              {Verb: "flee", Class: ClassMove, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"sneak":             {Verb: "sneak", Class: ClassSkill, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"pick":              {Verb: "pick", Class: ClassSkill, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"recall":            {Verb: "recall", Class: ClassSkill, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"persuade":          {Verb: "persuade", Class: ClassSocial, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"intimidate":        {Verb: "intimidate", Class: ClassSocial, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"command":           {Verb: "command", Class: ClassSocial, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"aid":               {Verb: "aid", Class: ClassSupport, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"mend":              {Verb: "mend", Class: ClassSupport, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"use_ability":       {Verb: "use_ability", Class: ClassResource, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
	"use_item":          {Verb: "use_item", Class: ClassResource, Rolls: true, Hard: true, Spends: false, Target: false, Node: false},
}

// classes — закрытый реестр классов. Существует потому, что класс приходит
// СНАРУЖИ: его предлагает парсер по свободному тексту игрока, и принимать его
// на слово нельзя. Выдуманное значение здесь не «неизвестный класс», а
// отсутствие подсказки.
var classes = map[VerbClass]bool{
	ClassNone: true, ClassInvestigate: true, ClassReason: true,
	ClassSocial: true, ClassMove: true, ClassAttack: true,
	ClassSupport: true, ClassResource: true, ClassSkill: true,
	ClassSave: true, ClassCheck: true, ClassRecover: true,
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
