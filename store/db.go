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
