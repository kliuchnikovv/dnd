// Package cases загружает рукописное дело из JSON и проверяет его инварианты.
package cases

import (
	"encoding/json"

	"github.com/kliuchnikovv/dnd/store"
)

// File — JSON-форма дела. Поля повторяют таблицы: файл читается как дамп базы,
// а не как отдельный формат со своей логикой.
type File struct {
	ID        store.CaseID `json:"id"`
	Archetype string       `json:"archetype"`
	Start     store.NodeID `json:"start"`
	Actor     string       `json:"actor"`

	Character struct {
		ID    string          `json:"id"`
		Grit  int             `json:"grit"`
		Harm  int             `json:"harm"`
		Sheet json.RawMessage `json:"sheet"`
	} `json:"character"`

	Locations      []store.Location      `json:"locations"`
	Entities       []store.Entity        `json:"entities"`
	Relations      []store.Relation      `json:"relations"`
	Facts          []store.Fact          `json:"facts"`
	FactHolders    []store.FactHolder    `json:"fact_holders"`
	FactUnlocks    []store.FactUnlock    `json:"fact_unlocks"`
	Contradictions []store.Contradiction `json:"contradictions"`
	Clocks         []store.Clock         `json:"clocks"`

	Tokens []struct {
		Slot  string       `json:"slot"`
		Token store.Token  `json:"token"`
		Fact  store.FactID `json:"fact"`
	} `json:"accusation_tokens"`

	Truth struct {
		Who  store.Token `json:"who"`
		How  store.Token `json:"how"`
		When store.Token `json:"when"`
		Why  store.Token `json:"why"`
	} `json:"truth"`

	StartFacts []struct {
		Fact store.FactID   `json:"fact"`
		From store.EntityID `json:"from"`
	} `json:"start_facts"`

	Flavour map[string]string `json:"flavour"`
}
