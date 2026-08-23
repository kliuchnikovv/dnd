package core

import "github.com/kliuchnikovv/dnd/store"

// Args — однородные аргументы всех глаголов. Незаполненные поля пусты;
// какие именно нужны, решает валидация конкретного глагола.
//
// JSON-теги здесь и на Intent — контракт журнала действий (ADR-0002): правда
// реплея это структурный интент, а не сырой текст игрока, поэтому имена его
// полей обязаны переживать переименования в коде.
type Args struct {
	Target   store.EntityID `json:"target,omitempty"`
	Topic    store.FactID   `json:"topic,omitempty"`
	Node     store.NodeID   `json:"node,omitempty"`
	Facts    []store.FactID `json:"facts,omitempty"`
	Item     string         `json:"item,omitempty"`
	Ability  string         `json:"ability,omitempty"`
	TagClaim string         `json:"tag_claim,omitempty"`
	Text     string         `json:"text,omitempty"`
}

type Intent struct {
	Verb  Verb              `json:"verb"`
	Actor store.CharacterID `json:"actor,omitempty"`
	Args  Args              `json:"args,omitempty"`
	// Push — заявка игрока потратить push-ресурс на этот бросок. Ядро не
	// знает, что это за ресурс и сколько он даёт: имя и цену назначает
	// система правил. Ядро знает только то, что решение приняли до броска.
	Push bool `json:"push,omitempty"`
}
