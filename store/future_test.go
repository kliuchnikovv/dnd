package store

import "testing"

// Таблицы, которых M1a не касается. Пустая таблица стоит ноль, а миграция
// потом стоит дорого: строки заводятся сейчас, чтобы переезд в Postgres
// остался механическим INSERT.
func TestFutureTablesExistAndStartEmpty(t *testing.T) {
	db := NewDB()
	if n := len(db.Cases); n != 0 {
		t.Errorf("cases не пуста: %d", n)
	}
	if n := len(db.Regions); n != 0 {
		t.Errorf("regions не пуста: %d", n)
	}
	if n := len(db.WorldEvents); n != 0 {
		t.Errorf("world_events не пуста: %d", n)
	}
	if n := len(db.Outbox); n != 0 {
		t.Errorf("outbox не пуста: %d", n)
	}
}

// Два поля, которые M1-документ прямо называет дорогими потом. Держателем
// бывает не только NPC, а сила связи нужна графу независимости источников,
// как только у него появится больше одного порога.
func TestExpensiveLaterFieldsAreLaidDownNow(t *testing.T) {
	h := FactHolder{HolderKind: "record"}
	if h.HolderKind != "record" {
		t.Error("fact_holders.holder_kind отсутствует")
	}
	r := Relation{Strength: 2}
	if r.Strength != 2 {
		t.Error("relations.strength отсутствует")
	}
}

// Вердикт живёт отдельно от правды: он попадает в мир и может быть неверным,
// а правда остаётся в деле. Другая парти опрокидывает именно вердикт.
func TestCaseRowSeparatesVerdictFromTruth(t *testing.T) {
	c := Case{ID: "harbour", Status: "closed", VerdictBy: "party", VerdictCorrect: false}
	if c.VerdictCorrect {
		t.Error("ошибочный вердикт записан как верный")
	}
	if c.VerdictBy == "" {
		t.Error("вердикт без парти — некому его опрокидывать")
	}
}
