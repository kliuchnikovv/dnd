package view

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

// TestOptionTokenStablePerIntent — токен опции детерминирован и привязан к
// интенту: один и тот же ход даёт один и тот же токен, разные ходы — разные.
// Это и есть контракт токена: клиент шлёт его обратно, сервер разворачивает
// в тот самый интент.
func TestOptionTokenStablePerIntent(t *testing.T) {
	a := core.Affordance{Intent: core.Intent{Verb: "examine", Args: core.Args{Target: "e_body"}}}
	b := core.Affordance{Intent: core.Intent{Verb: "examine", Args: core.Args{Target: "e_body"}}}
	c := core.Affordance{Intent: core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_body"}}}

	if OptionToken(a) == "" {
		t.Fatal("токен пуст")
	}
	if OptionToken(a) != OptionToken(b) {
		t.Errorf("один ход — разные токены: %q vs %q", OptionToken(a), OptionToken(b))
	}
	if OptionToken(a) == OptionToken(c) {
		t.Errorf("разные ходы — один токен: %q", OptionToken(a))
	}
}

// TestBuildFillsOptionTokens — сборка проставляет токен каждой опции: без него
// клиенту нечего слать обратно.
func TestBuildFillsOptionTokens(t *testing.T) {
	g := buildGame(t)
	tv := Build(g, core.TurnResult{}, nil, noRule, noScenario, "")
	if len(tv.Options) == 0 {
		t.Fatal("опций нет")
	}
	for i, o := range tv.Options {
		if o.Token == "" {
			t.Errorf("опция %d без токена: %+v", i, o)
		}
	}
}
