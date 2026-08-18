package core

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

const (
	MutResource    MutationKind = "resource"
	MutClock       MutationKind = "clock"
	MutHarm        MutationKind = "harm"
	MutDisposition MutationKind = "disposition"
	MutPosition    MutationKind = "position"
)

// Mutation обобщена намеренно: «слот 3 уровня» — это {resource, "slot_3", -1},
// а не поле в структуре ядра. Имена ресурсов даёт система правил.
type Mutation struct {
	Kind   MutationKind
	Target string
	Delta  int
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
