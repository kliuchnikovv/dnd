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
	// быт, фон. Payload — Text. Применяется через ProposeMutation, пишет
	// только в канон и никогда в факты дела.
	MutCanonAmbient MutationKind = "canon_ambient"
	// MutWorldEvent — событие слоя мира. Объявлен формой; обработчика нет и
	// не будет до серверного слоя с воркером мира.
	MutWorldEvent MutationKind = "world_event"
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
