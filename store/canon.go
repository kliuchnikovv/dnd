package store

import "sort"

// CanonKey — ключ ambient-канона: дело плюс тема. Канон растёт по ходу игры,
// но читается детерминированно — точным совпадением темы, а не поиском по
// векторам.
type CanonKey struct {
	CaseID CaseID
	Topic  string
}

// CanonFact — ambient-деталь мира, которой в авторской затравке не было.
// Автор у неё один — Мастер; персонажи её только озвучивают.
//
// Отдельная таблица от фактов дела, и это принципиально: факт дела гейтится
// fact_holders и не канонизируется импровизацией никогда, ambient-деталь
// решается на месте и запоминается. Спросят второй раз — вернётся то же, а не
// новая выдумка.
//
// Это же субстрат общего мира: канон, который расширил один прогон, виден
// следующему. Форма — под store/future.go: WorldEvent с этим payload.
type CanonFact struct {
	CaseID CaseID `json:"case_id"`
	Topic  string `json:"topic"`
	Text   string `json:"text"`
	// Turn — на каком ходу деталь стала каноном.
	Turn int `json:"turn"`
}

func (c CanonFact) Key() CanonKey {
	return CanonKey{CaseID: c.CaseID, Topic: c.Topic}
}

// CanonOf возвращает канон дела в стабильном порядке по теме. Итерация по map
// случайна, а промпт обязан собираться одинаково.
func (db *DB) CanonOf(c CaseID) []CanonFact {
	var out []CanonFact
	for _, f := range db.Canon {
		if f.CaseID == c {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Topic < out[j].Topic })
	return out
}
