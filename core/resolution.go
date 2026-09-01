package core

import "github.com/kliuchnikovv/dnd/store"

// Outcome — класс исхода. Порядок значений задаёт ступени: нат-20 поднимает
// на ступень, нат-1 опускает.
type Outcome int

const (
	OutcomeFail Outcome = iota
	OutcomePartial
	OutcomeSuccess
	OutcomeCrit
)

func (o Outcome) Up() Outcome {
	if o == OutcomeCrit {
		return OutcomeCrit
	}
	return o + 1
}

func (o Outcome) Down() Outcome {
	if o == OutcomeFail {
		return OutcomeFail
	}
	return o - 1
}

func (o Outcome) String() string {
	switch o {
	case OutcomeCrit:
		return "КРИТ"
	case OutcomeSuccess:
		return "УСПЕХ"
	case OutcomePartial:
		return "ЧАСТИЧНО"
	default:
		return "ПРОВАЛ"
	}
}

// CostKind — закрытая таксономия цены провала. Система правил выбирает
// элементы отсюда; исполняет их ядро. Ни одна сторона не делает обе вещи.
type CostKind string

const (
	CostTickClock       CostKind = "tick_clock"
	CostFalseLead       CostKind = "false_lead"
	CostDebt            CostKind = "debt"
	CostDispositionDown CostKind = "disposition_down"
	CostPositionWorse   CostKind = "position_worse"
	CostHarmSelf        CostKind = "harm_self"
	CostResourceSpent   CostKind = "resource_spent"
	CostHalfEffect      CostKind = "half_effect"
)

func AllCostKinds() []CostKind {
	return []CostKind{
		CostTickClock, CostFalseLead, CostDebt, CostDispositionDown,
		CostPositionWorse, CostHarmSelf, CostResourceSpent, CostHalfEffect,
	}
}

type MutationKind string

// Набор видов закрыт и делится на два пути, и деление здесь не косметическое.
// Первые пять приходят от системы правил — доверенный путь, applyMutations
// применяет их как есть. Остальные приходят от того, кто ядру не доверен
// (нарратор, куратор, воркер мира): их вход — только ProposeMutation, где вид
// проверяется, а незнакомое получает отказ, а не тишину. Вида, меняющего
// факты дела, party_knowledge, токены правды или механику броска, в этом
// наборе нет — и это гарантия конструкцией: нельзя предложить то, чего не
// существует.
const (
	MutResource    MutationKind = "resource"
	MutClock       MutationKind = "clock"
	MutHarm        MutationKind = "harm"
	MutDisposition MutationKind = "disposition"
	MutPosition    MutationKind = "position"

	// MutCanonAmbient — ambient-деталь мира по ключу (case, topic): погода,
	// быт, фон. Target — тема, Text — ответ, Delta — ход, на котором деталь
	// стала каноном. Применяется через ProposeMutation, пишет только в канон
	// и никогда в факты дела.
	MutCanonAmbient MutationKind = "canon_ambient"
	// MutPlaceKnown — место, о котором персонаж рассказал игроку. Target это
	// узел, Text не используется. Пишет в known_places и никогда в факты дела:
	// место — публичная география, оно ничего не доказывает.
	MutPlaceKnown MutationKind = "place_known"
	// MutWorldEvent — событие слоя мира. Объявлен формой; обработчика нет и
	// не будет до серверного слоя с воркером мира.
	MutWorldEvent MutationKind = "world_event"

	// MutHPDelta — изменение HP сущности. Target — сущность, Amount — дельта
	// (отрицательная — урон, положительная — лечение). Клампится в apply по
	// [0, MaxHP], если MaxHP задан.
	MutHPDelta MutationKind = "hp_delta"
	// MutSetCondition — состояние-тег на сущности (prone, poisoned, restrained).
	// Target — сущность, Condition — метка, Amount — TTL в раундах; Amount == 0
	// снимает условие.
	MutSetCondition MutationKind = "set_condition"
	// MutEncounterStart — вход в бой. Order задаёт порядок ходов на весь бой.
	MutEncounterStart MutationKind = "encounter_start"
	// MutEncounterEnd — выход из боя: Game.Encounter обнуляется.
	MutEncounterEnd MutationKind = "encounter_end"
	// MutAdvanceInitiative — конец такта текущей сущности: следующий ход.
	MutAdvanceInitiative MutationKind = "advance_initiative"
)

// Mutation обобщена намеренно: «слот 3 уровня» — это {resource, "slot_3", -1},
// а не поле в структуре ядра. Имена ресурсов даёт система правил.
type Mutation struct {
	Kind   MutationKind
	Target string
	Delta  int
	// Text — текстовый payload для видов, которые меняют не число, а слово:
	// канон мира. Числовые виды его не читают.
	Text string
	// Amount — числовой параметр боевых видов: HP-дельта, длительность
	// условия в раундах. Отдельно от Delta: Delta — язык ресурсов/часов/урона
	// по общей таксономии, Amount — язык боя, введённый позже и другой
	// семантикой (TTL, а не «плюс-минус против нуля»).
	Amount int
	// Condition — метка условия для MutSetCondition (prone, poisoned, ...).
	Condition string
	// Order — порядок ходов при MutEncounterStart.
	Order []store.EntityID
}

type RollTerm struct {
	Name  string
	Value int
}

// RollLog — единственный способ, которым значение броска попадает наружу:
// для показа игроку. Ядро им не управляет.
type RollLog struct {
	Die       int
	Terms     []RollTerm
	Total     int
	Threshold int
}

type Resolution struct {
	Class     Outcome
	Margin    int
	Costs     []CostKind
	Mutations []Mutation
	Log       RollLog
}

// Dice объявлен здесь, но реализован в пакете dice: core обязан оставаться
// свободным от math/rand и от словаря конкретной игральной кости.
type Dice interface {
	Roll(n, sides int) int
}

// RuleSystem — шов. Единственная точка, где ядро обращается к правилам.
type RuleSystem interface {
	Resolve(Intent, SceneView, Dice) Resolution
}
