package cli

import (
	"context"
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
// words — слова Мастера; пусто либо не той длины означает кодовые. Длина
// сверяется и здесь, хотя её уже сверила сессия: печать — последнее место,
// где строка может съехать на соседний интент, и второй раз это дешевле, чем
// один раз не проверить.
//
// Номера — способ ввода, а не разметка: под номером и под словами игрока лежит
// один и тот же путь применения. Ведущего тире здесь нет намеренно — строка,
// начинающаяся с тире, разбирается как прямая речь.
func (r Render) Affordances(g *core.Game, list []core.Affordance, words []string) string {
	if len(list) == 0 {
		return ""
	}
	if len(words) != len(list) {
		words = nil
	}
	var b strings.Builder
	b.WriteString("Что можно:\n")
	for i, a := range list {
		label := AffordanceLabel(g, a)
		if words != nil {
			label = words[i]
		}
		if a.Reply {
			label = Quoted(label)
		}
		fmt.Fprintf(&b, "  %d. %s\n", i+1, label)
	}
	b.WriteString(affordancePrompt)
	return b.String()
}

// Quoted берёт реплику в кавычки. Уже закавыченную не удваивает: Мастера
// просили писать без кавычек, но просьба — не гарантия, а две пары подряд
// увидит игрок, а не тест.
//
// Экспортирован по той же причине, что и AffordanceLabel: полноэкранный режим
// рисует набор своей панелью, и он обязан быть тем же выводом, а не вторым его
// форматом — значит, кавычки ставятся здесь же, одной функцией на оба приёмника.
func Quoted(line string) string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "«") && strings.HasSuffix(line, "»") {
		return line
	}
	return "«" + line + "»"
}

// AffordanceLabel — вариант словами. Имена берутся из тех же таблиц, что у
// списка целей: разойдись они, игрок читал бы про «e_bern».
//
// Экспортирован для полноэкранного режима: он рисует строки сам, потому что
// подсвечивает выбранную. Нумерацию и рамку списка при этом даёт Affordances —
// два места печати одного и того же разошлись бы.
func AffordanceLabel(g *core.Game, a core.Affordance) string {
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
		return shortName(e.Name)
	}
	if p, ok := g.DB.PropAt(g.Node, store.PropID(id)); ok {
		return shortName(p.Name)
	}
	return string(id)
}

// shortName — имя без пояснения после запятой. В списке целей «Берн, стражник»
// читается как надо, но во фразе получается «заговорить с Берн, стражник»:
// пояснение стоит в именительном и с фразой не согласуется. Отрезается именно
// оно, а не любая запятая, — и только здесь: в перечне пояснение полезно.
func shortName(name string) string {
	if i := strings.Index(name, ","); i > 0 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

func itemName(g *core.Game, id string) string {
	if item, ok := g.Item(store.ItemID(id)); ok {
		return item.Name
	}
	return id
}

// Option — вариант, которому нужны слова. Копия формы, а не структура master:
// cli не импортирует надстройки, иначе направление слоёв развернулось бы.
type Option struct {
	Text  string
	Reply bool
}

// OptionVoicer — необязательные слова для набора. Без него печатаются кодовые:
// игра без моделей обязана работать как работала.
type OptionVoicer interface {
	VoiceOptions(ctx context.Context, opts []Option) ([]string, error)
}

// optionsFor — набор в форме заказа на слова.
func optionsFor(g *core.Game, list []core.Affordance) []Option {
	out := make([]Option, 0, len(list))
	for _, a := range list {
		out = append(out, Option{Text: AffordanceLabel(g, a), Reply: a.Reply})
	}
	return out
}

// optionsKey — отпечаток набора. Слова просятся только когда набор сменился:
// он часто повторяется от хода к ходу, и повтор платить не должен.
func optionsKey(list []core.Affordance) string {
	var b strings.Builder
	for _, a := range list {
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s;", a.Intent.Verb, a.Intent.Args.Target,
			a.Intent.Args.Topic, a.Intent.Args.Node, a.Intent.Args.Item)
	}
	return b.String()
}
