package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func TestEncounterAdvanceWraps(t *testing.T) {
	e := &Encounter{Order: []store.EntityID{"a", "b", "c"}}
	if e.Current() != "a" {
		t.Fatalf("initial current: %q", e.Current())
	}
	e.Advance()
	if e.Current() != "b" {
		t.Fatalf("after 1 advance: %q", e.Current())
	}
	e.Advance()
	e.Advance()
	if e.Current() != "a" {
		t.Fatalf("wrap-around вернуть в a: %q", e.Current())
	}
	if e.Round != 1 {
		t.Fatalf("Round после полного круга: %d", e.Round)
	}
}

func TestEncounterResetClearsActions(t *testing.T) {
	e := &Encounter{Order: []store.EntityID{"a"}}
	e.Actions.Action, e.Actions.Bonus = true, true
	e.Reset()
	if e.Actions.Action || e.Actions.Bonus {
		t.Fatalf("после Reset флаги действий должны быть false: %+v", e.Actions)
	}
}
