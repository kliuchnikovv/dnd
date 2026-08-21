package store

import "sort"

// DB — набор таблиц в памяти. Ни одного указателя между сущностями:
// связи выражены идентификаторами, как в реляционной схеме.
type DB struct {
	Facts      map[FactID]Fact
	Holders    map[FactID][]FactHolder
	Unlocks    map[FactID][]FactUnlock
	Entities   map[EntityID]Entity
	Locations  map[NodeID]Location
	Relations  []Relation
	Clocks     map[ClockID]*Clock
	Characters map[CharacterID]*Character
	Knowledge  []Knowledge
	Dossiers   map[DossierKey]*Dossier
	Props      map[NodeID][]SceneProp
	// Canon — ambient-детали мира, решённые Мастером по ходу игры. Отдельно
	// от фактов дела: факт дела импровизацией не канонизируется никогда.
	Canon map[CanonKey]CanonFact

	// Пустые до M2. См. store/future.go: пустая таблица стоит ноль, миграция
	// потом стоит дорого.
	Cases       map[CaseID]*Case
	Regions     map[RegionID]Region
	WorldEvents []WorldEvent
	Outbox      []OutboxMessage

	Contradictions []Contradiction
}

func NewDB() *DB {
	return &DB{
		Facts:      map[FactID]Fact{},
		Holders:    map[FactID][]FactHolder{},
		Unlocks:    map[FactID][]FactUnlock{},
		Entities:   map[EntityID]Entity{},
		Locations:  map[NodeID]Location{},
		Clocks:     map[ClockID]*Clock{},
		Characters: map[CharacterID]*Character{},
		Dossiers:   map[DossierKey]*Dossier{},
		Props:      map[NodeID][]SceneProp{},
		Canon:      map[CanonKey]CanonFact{},
		Cases:      map[CaseID]*Case{},
		Regions:    map[RegionID]Region{},
	}
}

func (db *DB) HoldersOf(f FactID) []FactHolder { return db.Holders[f] }

// Related сообщает, есть ли ребро между двумя сущностями. Ребро
// ненаправленное: двое, кто общается, за два независимых источника не считаются
// вне зависимости от того, кто в JSON записан слева.
func (db *DB) Related(a, b EntityID) bool {
	for _, r := range db.Relations {
		if (r.From == a && r.To == b) || (r.From == b && r.To == a) {
			return true
		}
	}
	return false
}

func (db *DB) Adjacent(from, to NodeID) bool {
	for _, n := range db.Locations[from].Adjacent {
		if n == to {
			return true
		}
	}
	return false
}

// EntitiesAt возвращает сущности узла в стабильном порядке по ID.
// Итерация по map в Go случайна, а вывод сцены обязан быть воспроизводим.
func (db *DB) EntitiesAt(n NodeID) []Entity {
	var out []Entity
	for _, e := range db.Entities {
		if e.Node == n {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// DossierFor возвращает дневник сущности для парти, создавая его при первом
// обращении. Мировой слой (пустая парти) и отношенческий — разные строки.
func (db *DB) DossierFor(e EntityID, party string) *Dossier {
	key := DossierKey{EntityID: e, PartyID: party}
	if d, ok := db.Dossiers[key]; ok {
		return d
	}
	d := &Dossier{EntityID: e, PartyID: party, Kind: "npc"}
	db.Dossiers[key] = d
	return d
}

// WorldDossier возвращает мировой слой, если он есть.
func (db *DB) WorldDossier(e EntityID) (*Dossier, bool) {
	d, ok := db.Dossiers[DossierKey{EntityID: e}]
	return d, ok
}

// PropAt ищет проп по идентификатору в конкретном узле. Пропы не переезжают,
// поэтому узел — часть ключа поиска, а не фильтр после него.
func (db *DB) PropAt(n NodeID, id PropID) (SceneProp, bool) {
	for _, p := range db.Props[n] {
		if p.ID == id {
			return p, true
		}
	}
	return SceneProp{}, false
}
