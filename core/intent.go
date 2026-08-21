package core

import "github.com/kliuchnikovv/dnd/store"

// Args — однородные аргументы всех глаголов. Незаполненные поля пусты;
// какие именно нужны, решает валидация конкретного глагола.
type Args struct {
	Target   store.EntityID
	Topic    store.FactID
	Node     store.NodeID
	Facts    []store.FactID
	Item     string
	Ability  string
	TagClaim string
	Text     string
}

type Intent struct {
	Verb  Verb
	Actor store.CharacterID
	Args  Args
	// Push — заявка игрока потратить push-ресурс на этот бросок. Ядро не
	// знает, что это за ресурс и сколько он даёт: имя и цену назначает
	// система правил. Ядро знает только то, что решение приняли до броска.
	Push bool
}
