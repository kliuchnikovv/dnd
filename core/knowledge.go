package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

const (
	// SourceConfidence — вклад одного источника.
	SourceConfidence = 0.5
	// CorroborationThreshold — порог действенности. При p=0.5 достигается
	// тремя независимыми источниками: 1-0.5^3 = 0.875.
	CorroborationThreshold = 0.8
)

// Knowledge — таблица party_knowledge и операции над ней.
type Knowledge struct {
	db   *store.DB
	tick int
}

func NewKnowledge(db *store.DB) *Knowledge { return &Knowledge{db: db} }

// Learn записывает свидетельство. Возвращает false, если этот источник уже
// свидетельствовал об этом факте: ключ таблицы — пара (факт, источник).
func (k *Knowledge) Learn(f store.FactID, from store.EntityID) bool {
	for _, row := range k.db.Knowledge {
		if row.FactID == f && row.LearnedFrom == from {
			return false
		}
	}
	k.tick++
	k.db.Knowledge = append(k.db.Knowledge, store.Knowledge{
		FactID:      f,
		LearnedFrom: from,
		Confidence:  SourceConfidence,
		LearnedAt:   k.tick,
	})
	return true
}

func (k *Knowledge) Knows(f store.FactID) bool { return len(k.Sources(f)) > 0 }

// Sources возвращает источники в хронологическом порядке получения.
func (k *Knowledge) Sources(f store.FactID) []store.EntityID {
	rows := make([]store.Knowledge, 0, 4)
	for _, row := range k.db.Knowledge {
		if row.FactID == f {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].LearnedAt < rows[j].LearnedAt })
	out := make([]store.EntityID, len(rows))
	for i, r := range rows {
		out[i] = r.LearnedFrom
	}
	return out
}

// independent обходит источники в хронологическом порядке и оставляет те,
// у которых нет ребра ни с одним уже засчитанным. Обход детерминированный:
// перебора максимальных независимых множеств здесь нет и не нужно.
func (k *Knowledge) independent(f store.FactID) []store.EntityID {
	var kept []store.EntityID
	for _, cand := range k.Sources(f) {
		conflict := false
		for _, got := range kept {
			if k.db.Related(cand, got) {
				conflict = true
				break
			}
		}
		if !conflict {
			kept = append(kept, cand)
		}
	}
	return kept
}

// Confidence складывает независимые источники как 1 - П(1 - p).
func (k *Knowledge) Confidence(f store.FactID) float64 {
	product := 1.0
	for range k.independent(f) {
		product *= 1 - SourceConfidence
	}
	if product == 1.0 {
		return 0
	}
	return 1 - product
}

func (k *Knowledge) Corroborated(f store.FactID) bool {
	return k.Confidence(f) >= CorroborationThreshold
}

// TopicBank — единственный источник тем для вопросов. Спросить о факте,
// которого парти не знает, нельзя: это защита от угадывания.
func (k *Knowledge) TopicBank() []store.FactID {
	seen := map[store.FactID]bool{}
	var out []store.FactID
	for _, row := range k.db.Knowledge {
		if !seen[row.FactID] {
			seen[row.FactID] = true
			out = append(out, row.FactID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
