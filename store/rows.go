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
}

// Gate — условие выдачи факта держателем. Threshold — одно из
// "easy" | "normal" | "hard"; интерпретирует его система правил, не ядро.
type Gate struct {
	Verbs     []string `json:"verbs"`
	Threshold string   `json:"threshold"`
	Requires  []FactID `json:"requires"`
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
