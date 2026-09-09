package view

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

func TestLookupRulesetMissing(t *testing.T) {
	if _, ok := LookupRuleset("no-such-view-ruleset"); ok {
		t.Fatalf("незарегистрированный kind не должен находиться")
	}
}

func TestRegisterAndLookupRuleset(t *testing.T) {
	stub := fakeRuleset{meters: []Meter{{Label: "сентинель", Kind: "x", Value: 1, Surface: true}}}
	RegisterRuleset("test-view-ruleset", func() Ruleset { return stub })
	got, ok := LookupRuleset("test-view-ruleset")
	if !ok {
		t.Fatalf("зарегистрированный kind не нашёлся")
	}
	if len(got.Meters(nil)) != 1 || got.Meters(nil)[0].Label != "сентинель" {
		t.Fatalf("фабрика вернула не тот Ruleset: %+v", got.Meters(nil))
	}
	// core.RulesetKind — тип с ограниченным ассортиментом, но регистрация
	// должна принимать любой валидный идентификатор без вмешательства view.
	_ = core.RulesetThreshold
}
