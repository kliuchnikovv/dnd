package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func clocksDB() *store.DB {
	db := store.NewDB()
	db.Clocks["c_suspicion"] = &store.Clock{
		ID: "c_suspicion", Name: "Подозрение", Segments: 4, TickPolicy: "on_cost",
		OnFill: store.Consequence{FlavourKey: "clock.suspicion.filled", HostileTo: []store.EntityID{"e_toke"}},
	}
	return db
}

func TestTickFillsAndFiresOnce(t *testing.T) {
	c := NewClocks(clocksDB())
	if got := c.Tick("c_suspicion", 3); len(got) != 0 {
		t.Fatalf("часы сработали раньше заполнения: %v", got)
	}
	fired := c.Tick("c_suspicion", 1)
	if len(fired) != 1 || fired[0].FlavourKey != "clock.suspicion.filled" {
		t.Fatalf("последствие не сработало на заполнении: %v", fired)
	}
	// Повторные тики переполненных часов последствие не повторяют.
	if got := c.Tick("c_suspicion", 5); len(got) != 0 {
		t.Errorf("последствие сработало повторно: %v", got)
	}
	if !c.Filled("c_suspicion") {
		t.Error("часы не отмечены как заполненные")
	}
}

func TestTickNeverExceedsSegments(t *testing.T) {
	c := NewClocks(clocksDB())
	c.Tick("c_suspicion", 99)
	snap := c.Snapshot()
	if snap[0].Filled != snap[0].Segments {
		t.Errorf("filled=%d при segments=%d — счётчик убежал", snap[0].Filled, snap[0].Segments)
	}
}

func TestTickOnUnknownClockIsNoop(t *testing.T) {
	c := NewClocks(clocksDB())
	if got := c.Tick("c_nonexistent", 1); len(got) != 0 {
		t.Errorf("тик несуществующих часов дал последствие: %v", got)
	}
}
