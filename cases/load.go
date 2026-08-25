package cases

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// defaultParty — та же строка, которой пользуется ядро для одиночной игры.
// Ключ по парти закладывается с первого дня, чтобы кооп не требовал миграции.
const defaultParty = "party"

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
	for _, item := range f.Items {
		item.CaseID = f.ID
		db.Items[item.ID] = item
	}
	for _, id := range f.StartInventory {
		// Предмет, которого в деле нет, — опечатка автора, и видеть её надо на
		// загрузке. Молча положить его в инвентарь значит открыть гейт на
		// предмет, которого не существует.
		if _, ok := db.Items[id]; !ok {
			return nil, fmt.Errorf("cases: start_inventory ссылается на предмет %q, которого нет в items", id)
		}
		db.AddItem(defaultParty, id)
	}
	for _, p := range f.Props {
		db.Props[p.Node] = append(db.Props[p.Node], p)
		f.Flavour["prop."+string(p.ID)] = p.Text
	}
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

	// Кто держит факт, тот его знает. Выводится, а не пишется руками:
	// расхождение между fact_holders и дневником было бы багом, который
	// проявляется только в разговоре.
	for _, h := range f.FactHolders {
		world := db.DossierFor(h.HolderID, "")
		world.Kind = "npc"
		world.KnowsAbout = append(world.KnowsAbout, h.FactID)
	}

	// Дневник раскладывается на два слоя: объективное в мировой, отношение к
	// парти — в отношенческий. Разделение обязательно, иначе при коопе знание
	// разных парти склеится.
	for _, d := range f.Dossiers {
		world := db.DossierFor(d.Entity, "")
		world.Kind = "npc"
		world.Voice = d.Voice
		world.Life = d.Life
		world.KnowsAbout = append(world.KnowsAbout, d.KnowsAbout...)
		world.TalksAbout = append(world.TalksAbout, d.TalksAbout...)
		world.Wants = append(world.Wants, d.Wants...)

		rel := db.DossierFor(d.Entity, defaultParty)
		rel.Kind = "npc"
		rel.Disposition = d.Disposition
		rel.OpenThreads = append(rel.OpenThreads, d.OpenThreads...)
		rel.Summary = d.Summary
	}

	var tokens []core.TokenGrant
	for _, t := range f.Tokens {
		tokens = append(tokens, core.TokenGrant{Slot: t.Slot, Token: t.Token, Fact: t.Fact})
	}

	if err := validateFile(f); err != nil {
		return nil, err
	}

	return &core.Config{
		DB:          db,
		Truth:       accusation.NewTruth(f.Truth.Who, f.Truth.How, f.Truth.When, f.Truth.Why),
		Flavour:     f.Flavour,
		Setting:     f.Setting,
		CaseID:      f.ID,
		Tokens:      tokens,
		Start:       f.Start,
		Aftermath:   f.Aftermath,
		ColdCase:    f.ColdCase,
		Briefing:    f.Briefing,
		Hints:       f.Hints,
		Actor:       store.CharacterID(f.Actor),
		StartPlaces: f.StartPlaces,
	}, nil
}

// dossierKeyFor — ключ отношенческого слоя парти по умолчанию. Нужен тестам
// и вызывающим, которым иначе пришлось бы знать имя парти загрузчика.
func dossierKeyFor(e store.EntityID) store.DossierKey {
	return store.DossierKey{EntityID: e, PartyID: defaultParty}
}
