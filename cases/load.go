package cases

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func Load(path string) (*core.Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("чтение дела %s: %w", path, err)
	}
	cfg, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("разбор дела %s: %w", path, err)
	}
	return cfg, nil
}

func Parse(raw []byte) (*core.Config, error) {
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}

	db := store.NewDB()
	for _, l := range f.Locations {
		db.Locations[l.ID] = l
	}
	for _, e := range f.Entities {
		db.Entities[e.ID] = e
	}
	db.Relations = append(db.Relations, f.Relations...)
	for _, fact := range f.Facts {
		fact.CaseID = f.ID
		db.Facts[fact.ID] = fact
	}
	for _, h := range f.FactHolders {
		db.Holders[h.FactID] = append(db.Holders[h.FactID], h)
	}
	for _, u := range f.FactUnlocks {
		db.Unlocks[u.FactID] = append(db.Unlocks[u.FactID], u)
	}
	db.Contradictions = append(db.Contradictions, f.Contradictions...)
	for i := range f.Clocks {
		c := f.Clocks[i]
		db.Clocks[c.ID] = &c
	}
	db.Characters[store.CharacterID(f.Character.ID)] = &store.Character{
		ID:    store.CharacterID(f.Character.ID),
		Sheet: f.Character.Sheet,
		Grit:  f.Character.Grit,
		Harm:  f.Character.Harm,
	}
	// Стартовые факты пишутся прямо в party_knowledge: расследование начинается
	// не с пустого листа, иначе первый ход некуда сделать.
	for i, s := range f.StartFacts {
		db.Knowledge = append(db.Knowledge, store.Knowledge{
			FactID: s.Fact, LearnedFrom: s.From,
			Confidence: core.SourceConfidence, LearnedAt: i + 1,
		})
	}

	var tokens []core.TokenGrant
	for _, t := range f.Tokens {
		tokens = append(tokens, core.TokenGrant{Slot: t.Slot, Token: t.Token, Fact: t.Fact})
	}

	return &core.Config{
		DB:      db,
		Truth:   accusation.NewTruth(f.Truth.Who, f.Truth.How, f.Truth.When, f.Truth.Why),
		Flavour: f.Flavour,
		Tokens:  tokens,
		Start:   f.Start,
		Actor:   store.CharacterID(f.Actor),
	}, nil
}
