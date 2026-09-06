package core

import "testing"

func TestRulesetRegistryUnknownIsFalse(t *testing.T) {
	if _, ok := LookupRuleset("no-such"); ok {
		t.Fatal("registry соврал про незнакомый ruleset")
	}
}

func TestRulesetRegistryReturnsRegistered(t *testing.T) {
	stub := stubRules{}
	RegisterRuleset("test-ruleset", func() RuleSystem { return stub })
	got, ok := LookupRuleset("test-ruleset")
	if !ok || got == nil {
		t.Fatal("не вернул зарегистрированный")
	}
}

type stubRules struct{}

func (stubRules) Resolve(Intent, SceneView, Dice) Resolution { return Resolution{} }
