package store

import (
	"encoding/json"
	"testing"
)

func TestEntityJSONHasHPFields(t *testing.T) {
	raw := `{"id":"e_x","name":"X","kind":"npc","hp":10,"max_hp":10,"ac":13}`
	var e Entity
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e.HP != 10 || e.MaxHP != 10 || e.AC != 13 {
		t.Fatalf("HP/MaxHP/AC не разобрались: %+v", e)
	}
}

func TestEntityJSONWithoutHPFieldsIsZero(t *testing.T) {
	raw := `{"id":"e_x","name":"X","kind":"npc"}`
	var e Entity
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e.HP != 0 || e.MaxHP != 0 || e.AC != 0 {
		t.Fatalf("отсутствие полей должно давать 0: %+v", e)
	}
}
