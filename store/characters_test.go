package store

import "testing"

func TestSaveCharacterAndList(t *testing.T) {
	db := NewDB()
	db.SaveCharacter(&Character{ID: "chr_a", Ruleset: "threshold"})
	db.SaveCharacter(&Character{ID: "chr_b", Ruleset: "dnd5e"})
	got := db.Characters()
	if len(got) != 2 {
		t.Fatalf("Characters len=%d", len(got))
	}
	if got[0].ID != "chr_a" {
		t.Fatalf("order: %v", got)
	}
}

func TestCharacterByIDMissing(t *testing.T) {
	db := NewDB()
	if _, ok := db.CharacterByID("no"); ok {
		t.Fatal("несуществующий персонаж не должен возвращаться")
	}
}
