package store

import "encoding/json"

type FactKind string

const (
	FactConcept    FactKind = "concept"
	FactTransition FactKind = "transition"
)

type EntityKind string

const (
	EntityNPC    EntityKind = "npc"
	EntityRecord EntityKind = "record"
	EntityThing  EntityKind = "thing"
)

type Fact struct {
	ID     FactID   `json:"id"`
	CaseID CaseID   `json:"case_id"`
	Key    string   `json:"key"`
	Kind   FactKind `json:"kind"`
	// SummationClause — одна авторская строка развязки. Движок собирает из
	// таких строк речь игрока: состав и порядок задаёт он сам, поставив токены
	// в слоты. Нарратор в M2 получит готовый набор клауз и право только на
	// ритм — внести новую посылку он не сможет, потому что не знает ни одной.
	SummationClause string `json:"summation_clause"`
}

// Gate — условие выдачи факта держателем. Threshold — одно из
// "easy" | "normal" | "hard"; интерпретирует его система правил, не ядро.
type Gate struct {
	Verbs     []string     `json:"verbs"`
	Threshold string       `json:"threshold"`
	Requires  *Requirement `json:"requires"`
}

type FactHolder struct {
	FactID    FactID   `json:"fact_id"`
	HolderID  EntityID `json:"holder_id"`
	Gate      Gate     `json:"gate"`
	Mandatory bool     `json:"mandatory"`
	Latent    bool     `json:"latent"`
}

type FactUnlock struct {
	FactID      FactID `json:"fact_id"`
	UnlocksKind string `json:"unlocks_kind"` // "topic" | "node" | "entity"
	UnlocksID   string `json:"unlocks_id"`
}

type Entity struct {
	ID    EntityID   `json:"id"`
	Name  string     `json:"name"`
	Kind  EntityKind `json:"kind"`
	Voice string     `json:"voice"`
	Node  NodeID     `json:"node"`
}

type Location struct {
	ID       NodeID   `json:"id"`
	Name     string   `json:"name"`
	Adjacent []NodeID `json:"adjacent"`
	Tags     []string `json:"tags"` // dark, rain, crowd, indoors — читаются правилами
}

type Relation struct {
	From EntityID `json:"from_entity"`
	To   EntityID `json:"to_entity"`
	Kind string   `json:"kind"`
}

type Knowledge struct {
	FactID      FactID   `json:"fact_id"`
	LearnedFrom EntityID `json:"learned_from_entity"`
	Confidence  float64  `json:"confidence"`
	LearnedAt   int      `json:"learned_at"`
}

// Key — составной ключ таблицы party_knowledge.
func (k Knowledge) Key() [2]string { return [2]string{string(k.FactID), string(k.LearnedFrom)} }

type Clock struct {
	ID         ClockID `json:"id"`
	Name       string  `json:"name"`
	Segments   int     `json:"segments"`
	Filled     int     `json:"filled"`
	TickPolicy string  `json:"tick_policy"`
	// OnFill — ключ флейвора и мутации мира при заполнении. Часы не убивают
	// прогон, они меняют мир.
	OnFill Consequence `json:"on_fill"`
}

type Consequence struct {
	FlavourKey    string     `json:"flavour_key"`
	RemoveHolders []FactID   `json:"remove_holders"`
	HostileTo     []EntityID `json:"hostile_to"`
}

type Character struct {
	ID    CharacterID     `json:"id"`
	Sheet json.RawMessage `json:"sheet"` // непрозрачен для ядра
	Harm  int             `json:"harm"`
	Grit  int             `json:"grit"`
}

// Contradiction — пара фактов, чьё сопоставление открывает третий факт.
// Отношение симметрично: порядок аргументов compare роли не играет.
type Contradiction struct {
	A          FactID `json:"a"`
	B          FactID `json:"b"`
	Reveals    FactID `json:"reveals"`
	FlavourKey string `json:"flavour_key"`
}

// Requirement — предпосылки гейта с порогом: нужно не меньше N фактов из Of.
// Одна примитива покрывает три формы: все (N == len(Of)), любой (N == 1) и
// «любые K из M». Отдельных OR-гейтов поэтому не требуется.
type Requirement struct {
	Of []FactID `json:"of"`
	N  int      `json:"n"`
}

// RequireAll — предпосылки, которые нужны все. Для построения из Go.
func RequireAll(ids ...FactID) *Requirement {
	return &Requirement{Of: ids, N: len(ids)}
}

// RequireN — нужно не меньше n предпосылок из перечисленных.
func RequireN(n int, ids ...FactID) *Requirement {
	return &Requirement{Of: ids, N: n}
}

// UnmarshalJSON принимает две записи. Плоский массив — исторический вид, он
// означает «нужны все»; так продолжают читаться уже написанные дела. Объект
// {"of": [...], "n": k} задаёт порог; без "n" порог равен длине списка.
func (r *Requirement) UnmarshalJSON(data []byte) error {
	var flat []FactID
	if err := json.Unmarshal(data, &flat); err == nil {
		r.Of, r.N = flat, len(flat)
		return nil
	}
	var obj struct {
		Of []FactID `json:"of"`
		N  *int     `json:"n"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	r.Of = obj.Of
	if obj.N == nil {
		r.N = len(obj.Of)
	} else {
		r.N = *obj.N
	}
	return nil
}

// Satisfied сообщает, выполнены ли предпосылки. Знание о фактах передаётся
// предикатом: store остаётся без зависимостей от игровой логики.
func (r *Requirement) Satisfied(knows func(FactID) bool) bool {
	if r == nil || len(r.Of) == 0 {
		return true
	}
	got := 0
	for _, f := range r.Of {
		if knows(f) {
			got++
		}
	}
	return got >= r.N
}

// SceneProp — интерактивная деталь узла. Проп инертен: он никогда не держит
// факта, и это не договорённость, а разные таблицы. Смысл пропа — камуфляж:
// без него список целей узла состоит из одних холдеров и прямо выдаёт граф
// дела.
type SceneProp struct {
	ID   PropID   `json:"id"`
	Node NodeID   `json:"node_id"`
	Name string   `json:"name"`
	Kind string   `json:"kind"`
	Tags []string `json:"tags"`
	// Text — проза пробы. Один текст на все глаголы: разная проза на look и
	// examine намекала бы, что в пропе что-то есть.
	Text string `json:"text"`
}

// PropTags — закрытый словарь тегов пропа. Каждый тег обязан менять бросок:
// тег, которому нет строки в таблице ситуативных модификаторов, декоративен,
// и валидатор его не пропустит.
var PropTags = []string{"cover", "elevation", "dark", "crowd", "tool", "noise", "narrow"}

func KnownPropTag(tag string) bool {
	for _, t := range PropTags {
		if t == tag {
			return true
		}
	}
	return false
}
