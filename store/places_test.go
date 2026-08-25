package store

import "testing"

// Знание места — таблица, а не флаг у локации: одно и то же место известно
// одной парти и неизвестно другой, и в общем мире это станет обычным делом.
func TestKnownPlaceIsPerPartyAndCase(t *testing.T) {
	db := NewDB()
	db.KnowPlace("party", "harbour", "n_warehouse")

	if !db.KnowsPlace("party", "harbour", "n_warehouse") {
		t.Error("записанное место не читается")
	}
	if db.KnowsPlace("other", "harbour", "n_warehouse") {
		t.Error("знание одной парти видно другой")
	}
	if db.KnowsPlace("party", "forte_merlo", "n_warehouse") {
		t.Error("знание одного дела видно в другом")
	}
}

// Повтор — не ошибка: реплей и второй рассказ обязаны сходиться.
func TestKnowPlaceIsIdempotent(t *testing.T) {
	db := NewDB()
	db.KnowPlace("party", "harbour", "n_quay")
	db.KnowPlace("party", "harbour", "n_quay")
	if got := db.PlacesOf("party", "harbour"); len(got) != 1 {
		t.Errorf("повтор удвоил запись: %v", got)
	}
}

// Порядок стабилен: список мест печатается игроку и уезжает в промпт, а
// итерация по map случайна.
func TestPlacesReadInStableOrder(t *testing.T) {
	db := NewDB()
	for _, n := range []NodeID{"n_quay", "n_forge", "n_warehouse"} {
		db.KnowPlace("party", "harbour", n)
	}
	got := db.PlacesOf("party", "harbour")
	want := []NodeID{"n_forge", "n_quay", "n_warehouse"}
	if len(got) != len(want) {
		t.Fatalf("мест %d, ожидалось %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("порядок нестабилен: %v", got)
		}
	}
}
