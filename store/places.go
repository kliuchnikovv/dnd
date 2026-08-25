package store

import "sort"

// PlaceKey — ключ таблицы known_places: чья парти, какое дело, какое место.
// Дело в ключе потому же, что и у канона: место, известное в одном деле, не
// становится известным в другом.
type PlaceKey struct {
	PartyID string
	CaseID  CaseID
	NodeID  NodeID
}

// Знание мест — ОТДЕЛЬНАЯ таблица от party_knowledge, и это конструктивно.
// Место — публичная география: оно ничего не доказывает, токенов не открывает
// и в речь обвинения не входит. Держать его среди фактов дела значило бы
// стереть эту разницу и открыть недоверенному слою дверь к уликам.
func (db *DB) KnowsPlace(party string, c CaseID, n NodeID) bool {
	return db.KnownPlaces[PlaceKey{PartyID: party, CaseID: c, NodeID: n}]
}

// KnowPlace помечает место известным. Повтор — no-op: реплей и второй рассказ
// обязаны сходиться.
func (db *DB) KnowPlace(party string, c CaseID, n NodeID) {
	db.KnownPlaces[PlaceKey{PartyID: party, CaseID: c, NodeID: n}] = true
}

// PlacesOf — известные места в стабильном порядке. Итерация по map случайна, а
// список печатается игроку и уезжает в промпт парсера.
func (db *DB) PlacesOf(party string, c CaseID) []NodeID {
	var out []NodeID
	for k := range db.KnownPlaces {
		if k.PartyID == party && k.CaseID == c {
			out = append(out, k.NodeID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
