package core

import (
	"strings"

	"github.com/kliuchnikovv/dnd/store"
)

// applyMutations применяет обобщённые мутации. Ядро знает имена только двух
// ресурсов — grit и harm, потому что на них ссылается таксономия цены провала.
// Всё прочее приходит от системы правил и молча игнорируется, если ядру
// незнакомо: это не ошибка, а граница ответственности.
func (g *Game) applyMutations(ms []Mutation) {
	ch := g.DB.CharactersMap()[g.Actor]
	for _, m := range ms {
		switch m.Kind {
		case MutResource:
			if m.Target == "grit" && ch != nil {
				cur := ch.Grit
				cur = cur + m.Delta
				if cur < 0 {
					cur = 0
				}
				ch.Grit = cur
			}
		case MutHarm:
			if c, ok := g.DB.CharactersMap()[store.CharacterID(m.Target)]; ok {
				c.Harm = clampHarm(c.Harm + m.Delta)
			}
		case MutClock:
			g.C.Tick(store.ClockID(m.Target), m.Delta)
		case MutDisposition:
			g.D.Adjust(store.EntityID(m.Target), m.Delta)
		case MutPosition:
			g.Detected = m.Delta < 0
		case MutHPDelta:
			id := store.EntityID(m.Target)
			e := g.DB.Entities[id]
			hp := e.HP + m.Amount
			if hp < 0 {
				hp = 0
			}
			if e.MaxHP > 0 && hp > e.MaxHP {
				hp = e.MaxHP
			}
			e.HP = hp
			g.DB.Entities[id] = e
		case MutSetCondition:
			setCondition(g, m.Target, m.Condition, m.Amount)
		case MutEncounterStart:
			g.Encounter = &Encounter{Order: append([]store.EntityID(nil), m.Order...)}
		case MutEncounterEnd:
			g.Encounter = nil
		case MutAdvanceInitiative:
			if g.Encounter != nil {
				g.Encounter.Advance()
			}
		}
	}
}

// apply — точка входа одиночной мутации боевого пути. Обёртка над
// applyMutations: боевые виды идут тем же доверенным путём, что и
// resource/harm/clock — их источник система правил, а не недоверенный
// предложитель.
func apply(g *Game, m Mutation) {
	g.applyMutations([]Mutation{m})
}

// setCondition — состояние-тег на сущности с TTL в раундах. Хранится в
// store.DB.Conditions: EntityID → (метка → раундов осталось). Amount == 0
// снимает условие; иначе выставляет/перезаписывает TTL.
func setCondition(g *Game, target, condition string, amount int) {
	id := store.EntityID(target)
	if amount == 0 {
		if g.DB.Conditions != nil {
			delete(g.DB.Conditions[id], condition)
		}
		return
	}
	if g.DB.Conditions == nil {
		g.DB.Conditions = map[store.EntityID]map[string]int{}
	}
	if g.DB.Conditions[id] == nil {
		g.DB.Conditions[id] = map[string]int{}
	}
	g.DB.Conditions[id][condition] = amount
}

// executeCosts исполняет выбранную правилами цену. Возвращает последствия
// заполнившихся часов.
func (g *Game) executeCosts(cs []CostKind, in Intent) []store.Consequence {
	var fired []store.Consequence
	ch := g.DB.CharactersMap()[g.Actor]
	for _, c := range cs {
		switch c {
		case CostTickClock:
			fired = append(fired, g.C.TickAll(1)...)
		case CostDebt:
			g.Debts[in.Args.Target]++
		case CostDispositionDown:
			g.D.Adjust(in.Args.Target, -1)
		case CostPositionWorse:
			g.Detected = true
		case CostHarmSelf:
			if ch != nil {
				ch.Harm = clampHarm(ch.Harm + 1)
			}
		case CostFalseLead, CostHalfEffect, CostResourceSpent:
			// Меняют не мир, а исход хода: отражаются в TurnResult.
		}
	}
	return fired
}

// applyConsequences исполняет срабатывание часов. Держатели могут исчезнуть,
// сущности — озлобиться, но mandatory-путь к каждому факту дело обязано
// сохранить: это проверяет валидатор загрузки.
func (g *Game) applyConsequences(cs []store.Consequence) {
	for _, c := range cs {
		for _, f := range c.RemoveHolders {
			delete(g.DB.Holders, f)
		}
		for _, e := range c.HostileTo {
			g.D.Adjust(e, -2)
		}
	}
}

// clampHarm держит ранения в пределах листа: четвёртой ячейки не существует.
func clampHarm(h int) int {
	if h > HarmMax {
		return HarmMax
	}
	if h < 0 {
		return 0
	}
	return h
}

// Недоверенный вход в состояние (ADR-0001). Всё, что предлагает не система
// правил, а тот, кому ядро не доверяет — нарратор, куратор, воркер мира, —
// входит здесь и только здесь.
//
// От applyMutations этот путь отличается одним, и это главное: он всегда даёт
// вердикт. Незнакомое там — граница ответственности и законная тишина, здесь —
// отказ с причиной, потому что тишина в ответ на недоверенное предложение
// неотличима от применения.

// Applied — что ядро применило. Text держит ДЕЙСТВУЮЩЕЕ значение, а не
// предложенное: на занятой теме канон возвращает своё, и слой над доменом
// обязан озвучить именно его.
type Applied struct {
	Kind   MutationKind
	Target string
	Text   string
}

// Refusal — вердикт отказа. Пустая причина означает «не отказано»: два способа
// сказать одно разошлись бы, а различать их приходится в аудите.
type Refusal struct {
	Kind   MutationKind
	Reason string
}

func (r Refusal) Refused() bool { return r.Reason != "" }

// ProposeMutation — единственный вход мутаций для недоверенного предложителя.
// Проверяет инварианты; о ролях и капабилити не знает ничего — этот гейт живёт
// слоем выше, иначе ядро узнало бы про LLM.
func (g *Game) ProposeMutation(m Mutation) (Applied, Refusal) {
	switch m.Kind {
	case MutCanonAmbient:
		return g.proposeCanon(m)
	case MutPlaceKnown:
		return g.proposePlace(m)
	case MutResource, MutHarm, MutClock, MutDisposition, MutPosition:
		// Числа меняет только система правил после броска. Иначе нарратор
		// раздавал бы ранения словом.
		return refuseMutation(m, "числовую мутацию решает система правил, а не предложение")
	case MutWorldEvent:
		return refuseMutation(m, "события мира пока не исполняются")
	default:
		return refuseMutation(m, "неизвестный вид мутации")
	}
}

// proposeCanon канонизирует ambient-деталь. Target — тема, Text — ответ,
// Delta — ход, на котором деталь стала каноном.
func (g *Game) proposeCanon(m Mutation) (Applied, Refusal) {
	topic, text := canonTopic(m.Target), strings.TrimSpace(m.Text)
	if topic == "" || text == "" {
		return refuseMutation(m, "канон без темы или без ответа")
	}
	if g.factTopic(topic) {
		return refuseMutation(m, "тема принадлежит фактам дела — канон их не выдаёт")
	}
	if have, ok := g.CanonGet(topic); ok {
		// Тот же ответ на занятую тему — идемпотентность, а не ошибка: реплей
		// и переспрос обязаны сходиться. Иной ответ — отказ: канон держит
		// слово, и молча вернуть старое значило бы соврать предложителю, что
		// его версия принята.
		if have != text {
			return refuseMutation(m, "тема уже канонизирована иначе")
		}
		return Applied{Kind: MutCanonAmbient, Target: topic, Text: have}, Refusal{}
	}
	return Applied{
		Kind: MutCanonAmbient, Target: topic, Text: g.canonPut(topic, text, m.Delta),
	}, Refusal{}
}

// factTopic — принадлежит ли тема пространству имён фактов дела. Изоляция
// дела: иначе ambient-канон стал бы вторым способом выдать факт, минуя
// fact_holders, — и тем самым способом, от которого гейт и защищает.
//
// Отвергается и префикс идентификатора, и авторский ключ факта: предложить
// «f_toke_lied» и предложить «Токе соврал» — одна и та же попытка.
func (g *Game) factTopic(topic string) bool {
	if strings.HasPrefix(topic, factPrefix) {
		return true
	}
	for id, f := range g.DB.Facts {
		if f.CaseID != g.CaseID {
			continue
		}
		if canonTopic(string(id)) == topic || canonTopic(f.Key) == topic {
			return true
		}
	}
	return false
}

// factPrefix — префикс идентификаторов фактов дела. Живёт здесь, а не в
// разборе строк: это инвариант ядра, а не удобство ввода.
const factPrefix = "f_"

func refuseMutation(m Mutation, reason string) (Applied, Refusal) {
	return Applied{}, Refusal{Kind: m.Kind, Reason: reason}
}

// proposePlace делает место известным по рассказу персонажа.
//
// Смежность текущему узлу — единственное, что здесь стережёт карту: рассказать
// можно про то, куда отсюда ведёт дорога. Перепрыгнуть через полкарты одной
// репликой нельзя, и разведка остаётся занятием.
func (g *Game) proposePlace(m Mutation) (Applied, Refusal) {
	n := store.NodeID(m.Target)
	if _, ok := g.DB.Locations[n]; !ok {
		return refuseMutation(m, "такого места в деле нет")
	}
	if !g.DB.Adjacent(g.Node, n) {
		return refuseMutation(m, "отсюда дорога туда не ведёт")
	}
	// Повтор идемпотентен: рассказать дорогу второй раз не ошибка.
	g.knowPlace(n)
	return Applied{Kind: MutPlaceKnown, Target: string(n)}, Refusal{}
}
