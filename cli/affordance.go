package cli

import (
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// Слова для набора вариантов. Ядро говорит, что можно сделать; как это
// называется по-русски — здесь.
//
// Разделение не педантизм: оживить формулировку моделью можно будет, не тронув
// набор, — а значит, и не тронув ни его детерминизм, ни его leak-безопасность.

// classWords — класс проверки словами игрока. Печатается класс, а НЕ порог:
// число живёт в гейте держателя, то есть в данных дела, и напечатать его значило
// бы разметить, у каких целей есть авторский контент (ADR-0003, T2).
var classWords = map[core.VerbClass]string{
	core.ClassInvestigate: "расследование",
	core.ClassReason:      "рассуждение",
	core.ClassSocial:      "общение",
	core.ClassMove:        "перемещение",
	core.ClassAttack:      "схватка",
	core.ClassSupport:     "помощь",
	core.ClassResource:    "ресурс",
	core.ClassSkill:       "ловкость",
}

// affordancePrompt — строка под списком. Свободный ввод равноправен, и игрок
// обязан это видеть: список без неё читается как закрытое меню, то есть ровно
// как тот тупик, из которого эта ветка выбиралась.
const affordancePrompt = "  или напиши своими словами\n"

// Affordances печатает набор вариантов нумерованным списком.
//
// Номера — способ ввода, а не разметка: под номером и под словами игрока лежит
// один и тот же путь применения. Ведущего тире здесь нет намеренно — строка,
// начинающаяся с тире, разбирается как прямая речь.
func (r Render) Affordances(g *core.Game, list []core.Affordance) string {
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Что можно:\n")
	for i, a := range list {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, affordanceLabel(g, a))
	}
	b.WriteString(affordancePrompt)
	return b.String()
}

// affordanceLabel — вариант словами. Имена берутся из тех же таблиц, что у
// списка целей: разойдись они, игрок читал бы про «e_bern».
func affordanceLabel(g *core.Game, a core.Affordance) string {
	label := affordanceAction(g, a)
	if word, ok := classWords[a.Check]; ok {
		label += " [" + word + "]"
	}
	return label
}

func affordanceAction(g *core.Game, a core.Affordance) string {
	args := a.Intent.Args
	switch a.Intent.Verb {
	case "talk_to":
		return "заговорить с " + entityName(g, args.Target)
	case "question":
		// Открытый вопрос, без темы: человек расскажет то, что готов
		// рассказать. Названная тема в набор не попадает — см. core.
		return "расспросить " + entityName(g, args.Target)
	case "examine":
		return "осмотреть " + entityName(g, args.Target)
	case "move_zone":
		return "перейти: " + g.DB.Locations[args.Node].Name
	case "present":
		return "предъявить " + itemName(g, args.Item) + " — " + entityName(g, args.Target)
	default:
		// Незнакомый глагол печатается как есть. Молча пропустить вариант
		// значило бы показать игроку список короче того, что предложило ядро.
		return string(a.Intent.Verb) + " " + string(args.Target)
	}
}

// entityName — имя цели. Люди и детали места вместе: целью бывает и то и
// другое, и брать имена из двух разных мест значило бы получить «e_bern» ровно
// там, где список целей печатает «Берн, стражник».
func entityName(g *core.Game, id store.EntityID) string {
	if e, ok := g.DB.Entities[id]; ok {
		return e.Name
	}
	if p, ok := g.DB.PropAt(g.Node, store.PropID(id)); ok {
		return p.Name
	}
	return string(id)
}

func itemName(g *core.Game, id string) string {
	if item, ok := g.Item(store.ItemID(id)); ok {
		return item.Name
	}
	return id
}
