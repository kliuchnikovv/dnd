package dnd5e

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

func TestExtraVerbsIncludeAttackHideRest(t *testing.T) {
	got := Verbs()
	have := map[string]bool{}
	for _, v := range got {
		have[string(v.Verb)] = true
	}
	for _, name := range []string{"attack", "hide", "disarm_trap",
		"detect_trap", "pick_lock", "rest_short", "rest_long"} {
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

func TestDCOfIntentFallsBackToDefault(t *testing.T) {
	in := core.Intent{Verb: "hide"}
	view := core.SceneView{DC: 0}
	dc := DCOfIntent(in, view)
	if dc != DefaultDC {
		t.Errorf("DCOfIntent with no explicit DC: got %d, want %d", dc, DefaultDC)
	}
}
