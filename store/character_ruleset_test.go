package store

import (
	"encoding/json"
	"testing"
)

func TestCharacterJSONCarriesRuleset(t *testing.T) {
	raw := `{"id":"chr_kay","ruleset":"dnd5e","sheet":{}}`
	var c Character
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if c.Ruleset != "dnd5e" {
		t.Fatalf("ruleset: %q", c.Ruleset)
	}
}

func TestCharacterWithoutRulesetIsEmpty(t *testing.T) {
	raw := `{"id":"chr_x","sheet":{}}`
	var c Character
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if c.Ruleset != "" {
		t.Fatalf("должно быть пусто, получено: %q", c.Ruleset)
	}
}
