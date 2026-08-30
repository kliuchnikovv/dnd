package view

import (
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// Ruleset — ось ПРАВИЛА. Правило называет свои меры само: «раны», «grit»,
// «часы», «heat» подземелья — и решает по состоянию, какую сейчас показать
// (Surface). Интерфейс живёт здесь, реализация — в презентации: так вид
// остаётся свободным от слов конкретного правила, и шов агностичности честен.
type Ruleset interface {
	Meters(g *core.Game) []Meter
}

// Scenario — ось СЦЕНАРИЯ. Сценарий даёт панель цели: детективное досье,
// зачистка подземелья, добыча — экземпляры обобщённой панели. Реализация — в
// презентации, по той же причине, что и у правила.
type Scenario interface {
	Objective(g *core.Game) *Panel
}

// Build собирает вид одного хода из ядра, правила и сценария.
//
// Агностичную часть — сцену, резолюцию, опции, присутствующих, развязку —
// собирает сам сборщик из состояния ядра: она одинакова у любой пары
// (правило × сценарий). Ось-специфичное — меры и панель цели — приходит от
// правила и сценария через интерфейсы: слов «grit» или «casebook» сборщик не
// знает.
//
// narration — блоки прозы Мастера и реплики NPC. Пусты без моделей: игра без
// них обязана оставаться полной механикой, и меры/резолюция/опции собираются
// независимо от того, доехала ли проза.
//
// with — собеседник для набора реплик, тем же параметром, что у
// core.Affordances: память разговора пишет актёр, и восстанавливается реплеем.
func Build(g *core.Game, res core.TurnResult, narration []Block, rs Ruleset, sc Scenario, with store.EntityID) TurnView {
	tv := TurnView{
		Version:      version,
		Scene:        Scene{Node: string(g.Node), Title: g.DB.Locations[g.Node].Name},
		Narration:    narration,
		Resolution:   resolutionOf(res),
		Meters:       rs.Meters(g),
		Options:      optionsOf(g, with),
		Objective:    sc.Objective(g),
		Participants: participantsOf(g),
		Ended:        endingOf(g.Solved(), g.Stalled(), g.ColdCase()),
	}
	return tv
}

// version — версия формы вида. Отдельной константой, а не литералом в Build:
// поднять её придётся при первом же несовместимом сдвиге контракта, и место
// для этого должно быть одно.
const version = 1

// resolutionOf переводит бросок ядра в форму, свободную от словаря кости. Nil,
// если броска не было: у безопасного действия Log.Die равен нулю, и вид не
// несёт пустую резолюцию.
func resolutionOf(res core.TurnResult) *Resolution {
	if res.Res == nil {
		return nil
	}
	log := res.Res.Log
	terms := make([]Term, 0, len(log.Terms))
	for _, t := range log.Terms {
		terms = append(terms, Term{Label: t.Name, Value: t.Value})
	}
	return &Resolution{
		Terms:   terms,
		Target:  log.Threshold,
		Margin:  res.Res.Margin,
		Outcome: Outcome{Label: res.Res.Class.String(), Tier: tierOf(res.Res.Class)},
	}
}

// tierOf — машинная ступень исхода для клиента: он красит исход, но русских
// слов Class.String() не разбирает.
func tierOf(o core.Outcome) string {
	switch o {
	case core.OutcomeCrit:
		return "crit"
	case core.OutcomeSuccess:
		return "success"
	case core.OutcomePartial:
		return "partial"
	default:
		return "fail"
	}
}

// participantsOf — люди сцены. Вещи (труп, деталь) сюда не входят: у аватара
// нет носителя, а присутствующий — это тот, с кем говорят.
func participantsOf(g *core.Game) []Who {
	var out []Who
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if e.Kind != store.EntityNPC {
			continue
		}
		out = append(out, Who{ID: string(e.ID), Name: e.Name, Disposition: g.D.Disposition(e.ID)})
	}
	return out
}

// endingOf — развязка по состоянию. Раскрытое дело важнее висяка: у игрока,
// раскрывшего дело на последнем ходу перед остановкой, исход один — успех.
// Висяк — только у остановившегося прогона с текстом: молчащий висяк читался бы
// как поломка.
func endingOf(solved, stalled bool, cold string) *Ending {
	if solved {
		return &Ending{Kind: "solved"}
	}
	if stalled && cold != "" {
		return &Ending{Kind: "cold", Text: cold}
	}
	return nil
}

// optionsOf строит опции из read-scope аффордансов ядра. Слова — презентация
// глаголов и классов ядра, не словарь правила или сценария: «осмотреть»,
// «расследование» одинаковы у детектива и подземелья, поэтому и живут здесь, а
// не за осевым интерфейсом. Токен опции ставит фаза 4.
func optionsOf(g *core.Game, with store.EntityID) []Option {
	list := g.Affordances(with)
	out := make([]Option, 0, len(list))
	for _, a := range list {
		out = append(out, Option{
			Label: affordanceLabel(g, a),
			Token: OptionToken(a),
			Check: classWords[a.Check],
			Reply: a.Reply,
		})
	}
	return out
}

// OptionToken — стабильный токен аффорданса. Детерминирован и привязан к
// интенту: клиент шлёт токен обратно, сервер разворачивает его в тот самый ход
// и ВСЁ РАВНО валидирует — авторитет применения на ядре, не на клиенте. Тот же
// набор полей, что у отпечатка набора, чтобы один ход всегда давал один токен,
// а позиция в списке на токен не влияла.
func OptionToken(a core.Affordance) string {
	in := a.Intent
	return strings.Join([]string{
		string(in.Verb), string(in.Args.Target), string(in.Args.Topic),
		string(in.Args.Node), in.Args.Item,
	}, "|")
}

// classWords — класс проверки словами игрока. Печатается класс, а НЕ порог:
// число живёт в гейте держателя, и напечатать его значило бы разметить, у каких
// целей есть авторский контент (ADR-0003, T2). Классы — таксономия ядра, не
// правила: их именование осью не является.
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

// AffordanceLabel — тот же лейбл варианта, что видит игрок в Option.Label.
// Экспортирован для транскрипта: сервер подписывает ход игрока ровно так, как
// вариант выглядел на экране.
func AffordanceLabel(g *core.Game, a core.Affordance) string {
	return affordanceLabel(g, a)
}

// affordanceLabel — вариант словами. Класс, если он есть, идёт в скобках после
// действия.
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
		return "расспросить " + entityName(g, args.Target)
	case "examine":
		return "осмотреть " + entityName(g, args.Target)
	case "move_zone":
		return "перейти: " + g.DB.Locations[args.Node].Name
	case "present":
		return "предъявить " + itemName(g, args.Item) + " — " + entityName(g, args.Target)
	default:
		// Незнакомый глагол печатается как есть: молча укоротить набор значило бы
		// показать список короче того, что предложило ядро.
		return string(a.Intent.Verb) + " " + string(args.Target)
	}
}

// entityName — имя цели: люди и детали места вместе, из тех же таблиц, что у
// списка целей. Разойдись они — игрок читал бы про «e_toke».
func entityName(g *core.Game, id store.EntityID) string {
	if e, ok := g.DB.Entities[id]; ok {
		return shortName(e.Name)
	}
	if p, ok := g.DB.PropAt(g.Node, store.PropID(id)); ok {
		return shortName(p.Name)
	}
	return string(id)
}

// shortName — имя без пояснения после запятой: во фразе «заговорить с Берн,
// стражник» пояснение стоит в именительном и не согласуется.
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
