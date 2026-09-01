package dnd5e

import "testing"

func TestExtraVerbsIncludeAttackHideRest(t *testing.T) {
	got := Verbs()
	have := map[string]bool{}
	for _, v := range got {
		have[string(v.Verb)] = true
	}
	for _, name := range []string{"attack", "hide", "sneak", "disarm_trap",
		"detect_trap", "pick_lock", "flee", "rest_short", "rest_long"} {
		if !have[name] {
			t.Errorf("верб %q не объявлен", name)
		}
	}
}

func TestSkillOfVerb(t *testing.T) {
	if SkillOfVerb("hide") != "stealth" {
		t.Errorf("hide → %q", SkillOfVerb("hide"))
	}
	if SkillOfVerb("pick_lock") != "thieves_tools" {
		t.Errorf("pick_lock → %q", SkillOfVerb("pick_lock"))
	}
}
