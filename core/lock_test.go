package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// Лок-на-успехе (прото §9, дух Take-20): успех записывает знание, и глубже по
// тому же факту не докопаться (держатель известного факта пропускается). Провал
// НЕ лочит — держатель остаётся доступен, повтор возможен, цена = время. Здесь
// это следствие идемпотентности знания, а не отдельного флага «locked».

func lockGame(out Outcome) *Game {
	db := store.NewDB()
	db.Locations["n_room"] = store.Location{ID: "n_room"}
	db.Entities["e_wit"] = store.Entity{ID: "e_wit", Kind: store.EntityNPC, Node: "n_room"}
	db.Facts["f_clue"] = store.Fact{ID: "f_clue", Key: "clue"}
	db.Holders["f_clue"] = []store.FactHolder{{
		FactID: "f_clue", HolderID: "e_wit",
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "normal"},
	}}
	return NewGame(Config{
		DB: db, Rules: fixedRules{out: out}, Dice: nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_room", Actor: "pc",
	})
}

func TestSuccessLocksTheAspect(t *testing.T) {
	g := lockGame(OutcomeSuccess)
	q := Intent{Verb: "examine", Args: Args{Target: "e_wit"}}

	first := g.Apply(q)
	if len(first.Learned) != 1 {
		t.Fatalf("успех не открыл факт: %v", first.Learned)
	}
	// Второй заход по тому же: докопаться глубже нечего — факт уже известен.
	second := g.Apply(q)
	if len(second.Learned) != 0 {
		t.Errorf("успех не запер аспект — факт выдан повторно: %v", second.Learned)
	}
}

func TestFailureDoesNotLockAndIsRetryable(t *testing.T) {
	g := lockGame(OutcomeFail)
	q := Intent{Verb: "examine", Args: Args{Target: "e_wit"}}

	if got := g.Apply(q); len(got.Learned) != 0 {
		t.Fatalf("провал выдал факт: %v", got.Learned)
	}
	if g.K.Knows("f_clue") {
		t.Fatal("провал записал знание — аспект заперт на провале (Take-20 сказал бы «вернись»)")
	}
	// Провал не запер: тот же держатель доступен, и на успехе факт открывается.
	g.Rules = fixedRules{out: OutcomeSuccess}
	if got := g.Apply(q); len(got.Learned) != 1 {
		t.Errorf("повтор после провала не смог открыть факт — провал залочил аспект: %v", got.Learned)
	}
}
