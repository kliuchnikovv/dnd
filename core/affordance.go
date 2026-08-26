package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

// Аффордансы — 2–4 варианта хода, которые игра предлагает игроку каждый ход.
//
// Смысл не в удобстве, а в том, чтобы показать, что здесь вообще уместно.
// Свободный ввод остаётся равноправным: это ускорители, а не меню.
//
// Набор строит КОД, а не модель, и это решение о безопасности, а не о цене.
// Утечка в подсказках живёт не в формулировке, а в самом наборе: список, где
// одна опция ведёт к разгадке, спойлерит безупречными словами. Здесь генератор
// не читает граф держателей вовсе — и «вариант, ведущий к разгадке» не может
// попасть в набор по построению, а не по аккуратности (ADR-0003, T2).
//
// Он же детерминирован: набор — часть той же правды, что (seed, script).

// Affordance — предложенный вариант хода.
//
// Слов здесь нет: формулировку даёт презентация, и это не педантизм. Ядро
// говорит, ЧТО можно сделать; как это назвать по-русски — вопрос того слоя,
// который печатает. Оживить формулировку моделью можно будет, не тронув набор.
type Affordance struct {
	Intent Intent
	// Check — класс проверки; пусто, если броска не будет.
	//
	// Класс, а не порог. Порог живёт в гейте держателя, то есть в данных дела:
	// напечатать число значило бы разметить, у каких целей есть авторский
	// контент. Класс выводится из реестра глаголов и о деле не знает ничего.
	Check VerbClass
}

// affordanceLimit — сколько вариантов показывается. Четыре, потому что список
// длиннее читается как меню и вытесняет то, ради чего он и нужен, — мысль о
// том, что можно написать своими словами.
const affordanceLimit = 4

// Affordances — набор вариантов из read scope: присутствующие, детали места,
// известные места, носимое.
//
// Тем среди источников нет — см. комментарий у открытого вопроса ниже.
//
// Категории идут в фиксированном приоритете и дают по одному варианту каждая.
// Приоритет — не суждение о ценности хода, а способ получить устойчивый и
// разнообразный набор: четыре осмотра подряд не показали бы игроку, что здесь
// вообще можно разговаривать.
//
// Ротации нет намеренно. Она потребовала бы счётчика в состоянии, а всё, что
// участвует в наборе, участвует и в воспроизводимости прогона.
func (g *Game) Affordances() []Affordance {
	var out []Affordance
	add := func(a Affordance, ok bool) {
		if !ok || len(out) >= affordanceLimit {
			return
		}
		a.Check = checkOf(a.Intent.Verb)
		a.Intent.Actor = g.Actor
		out = append(out, a)
	}

	// По одному варианту на человека, а не два на первого. «Заговорить с
	// Берном» и «расспросить Берна» рядом читаются как один ход, написанный
	// дважды: живой прогон получил их первыми двумя строками и не увидел
	// разницы, а второй присутствующий в набор не попал вовсе.
	npcs := g.npcsHere()
	for _, id := range npcs {
		add(Affordance{Intent: Intent{Verb: g.socialVerb(id),
			Args: Args{Target: id}}}, true)
	}

	prop, hasProp := g.firstProp()
	add(Affordance{Intent: Intent{Verb: "examine",
		Args: Args{Target: prop}}}, hasProp)

	node, hasNode := g.firstKnownPlace()
	add(Affordance{Intent: Intent{Verb: "move_zone",
		Args: Args{Node: node}}}, hasNode)

	item, hasItem := g.firstCarried()
	add(Affordance{Intent: Intent{Verb: "present",
		Args: Args{Item: item, Target: first(npcs)}}}, hasItem && len(npcs) > 0)

	return out
}

// socialVerb — с чего начать с этим человеком.
//
// Глагол идёт за разговором: предлагать «заговорить» тому, с кем беседа уже
// идёт, значит начинать её заново, а «расспросить» до знакомства — допрос с
// порога. Признак — собственная память разговора парти, то есть то, при чём
// игрок присутствовал.
//
// Вопрос ОТКРЫТЫЙ, без темы, и это отступление от первой редакции дизайна.
// Тема в вопросе принимается ядром только тогда, когда цель этот факт держит:
// вариант «спросить Берна про шнур» появлялся бы ровно там, где Берн про шнур
// знает, — и само его появление выдавало бы авторский контент. Альтернатива —
// предлагать ход, который ядро отклонит, то есть врать меню.
func (g *Game) socialVerb(id store.EntityID) Verb {
	if len(g.D.Recent(id)) > 0 {
		return "question"
	}
	return "talk_to"
}

// npcsHere — люди этого узла в стабильном порядке.
func (g *Game) npcsHere() []store.EntityID {
	var out []store.EntityID
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if e.Kind == store.EntityNPC {
			out = append(out, e.ID)
		}
	}
	return out
}

func first(ids []store.EntityID) store.EntityID {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// rollDecidedElsewhere — глаголы, у которых Rolls реестра последнего слова не
// говорит. use_ability и use_item перекрываются данными дела; move_zone не
// бросается вообще — система правил считает недошедший переход не ценой, а
// сбоем. Тег на таком варианте обещал бы проверку, которой не будет, а обещание
// в меню хуже отсутствия пометки.
//
// Расхождение реестра с системой правил по move_zone — известное и своё; здесь
// оно только не выносится игроку.
var rollDecidedElsewhere = map[Verb]bool{
	"move_zone": true, "use_ability": true, "use_item": true,
}

// checkOf — класс проверки варианта; пусто, если броска не будет.
func checkOf(v Verb) VerbClass {
	def, ok := Verbs[v]
	if !ok || !def.Rolls || rollDecidedElsewhere[v] {
		return ""
	}
	return def.Class
}

// firstProp — первая деталь узла. Держатели здесь НЕ смотрятся: отбор по ним
// разметил бы, где лежит авторский контент.
func (g *Game) firstProp() (store.EntityID, bool) {
	props := g.DB.Props[g.Node]
	if len(props) == 0 {
		return "", false
	}
	ids := make([]store.PropID, 0, len(props))
	for _, p := range props {
		ids = append(ids, p.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return store.EntityID(ids[0]), true
}

// firstKnownPlace — первое известное место, кроме текущего. ReachableNodes уже
// стоит на знании, а не на смежности, и второй раз этот гейт проверять негде.
func (g *Game) firstKnownPlace() (store.NodeID, bool) {
	reach := g.ReachableNodes()
	if len(reach) == 0 {
		return "", false
	}
	return reach[0], true
}

// firstCarried — первый носимый предмет.
func (g *Game) firstCarried() (string, bool) {
	carried := g.Carried()
	if len(carried) == 0 {
		return "", false
	}
	return string(carried[0].ID), true
}
