package dnd5e

import "testing"

func TestSkillAbilityMappingCovers5eBasics(t *testing.T) {
	cases := map[string]string{
		"athletics": "str", "acrobatics": "dex", "stealth": "dex",
		"perception": "wis", "insight": "wis", "investigation": "int",
		"persuasion": "cha", "deception": "cha", "thieves_tools": "dex",
	}
	for skill, want := range cases {
		if got := SkillAbility(skill); got != want {
			t.Errorf("%s → %q, ждали %q", skill, got, want)
		}
	}
}

func TestSkillAbilityUnknownIsEmpty(t *testing.T) {
	if got := SkillAbility("no-such"); got != "" {
		t.Errorf("неизвестный skill → %q, ждали пустую", got)
	}
}
