package dnd5e

import (
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

func TestCheckPassWhenRollBeatsDC(t *testing.T) {
	s := Sheet{Dex: 16, Prof: 2, Skills: []string{"stealth"}}
	// roll=7: 7 + Dex(3) + Prof(2) = 12 против DC 12 — успех на грани.
	res := check(s, "stealth", 12, 7)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("успех ожидался, получен %v (margin %d)", res.Class, res.Margin)
	}
	if res.Margin != 0 {
		t.Fatalf("margin: ждали 0, получили %d", res.Margin)
	}
}

func TestCheckFailWhenRollMissesDC(t *testing.T) {
	s := Sheet{Dex: 16, Prof: 2, Skills: []string{"stealth"}}
	// 6 + 3 + 2 = 11 против DC 12 — провал на 1.
	res := check(s, "stealth", 12, 6)
	if res.Class != core.OutcomeFail {
		t.Fatalf("провал ожидался, получен %v", res.Class)
	}
	if res.Margin != 1 {
		t.Fatalf("margin: ждали 1, получили %d", res.Margin)
	}
}

func TestCheckWithoutProficiencyOmitsProfBonus(t *testing.T) {
	s := Sheet{Dex: 16, Prof: 2} // stealth не в списке навыков
	// 10 + 3 (dex) = 13, без prof; DC 13 — ровно успех.
	res := check(s, "stealth", 13, 10)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("успех без prof ожидался, получен %v", res.Class)
	}
}

func TestCheckUnknownSkillFallsBackToFlatRoll(t *testing.T) {
	s := Sheet{Dex: 16, Prof: 2, Skills: []string{"no-such"}}
	// Неизвестный skill → ability "" → Mod("") == 0, Proficient("no-such")
	// не совпадает с пустым skill, поэтому итог — голый бросок.
	res := check(s, "", 10, 10)
	if res.Class != core.OutcomeSuccess || res.Margin != 0 {
		t.Fatalf("голый бросок 10 против DC 10: %v margin=%d", res.Class, res.Margin)
	}
}

// TestSystemResolveDispatchesCheckForSkillVerb проверяет весь путь через
// core.RuleSystem: verb "sneak" (ClassSkill, Rolls=true) должен уйти в
// resolveCheck и использовать SkillOfVerb + SceneView.DC.
func TestSystemResolveDispatchesCheckForSkillVerb(t *testing.T) {
	s := Sheet{Dex: 16, Prof: 2, Skills: []string{"stealth"}}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	view := core.SceneView{Sheet: raw, DC: 12}
	sys := New()
	d := dice.Fixed(7) // d20 = 7 → total 7+3+2=12 vs DC 12 — успех
	res := sys.Resolve(core.Intent{Verb: "sneak"}, view, d)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("успех ожидался через System.Resolve, получен %v", res.Class)
	}
}

func TestSystemResolveNoRollVerbAlwaysSucceeds(t *testing.T) {
	sys := New()
	res := sys.Resolve(core.Intent{Verb: "look"}, core.SceneView{}, dice.Fixed(1))
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("look не бросает — ожидался автоуспех, получен %v", res.Class)
	}
}
