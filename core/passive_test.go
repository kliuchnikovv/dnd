package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// passiveGame — дело с одним пассивно-гейтнутым tell'ом на предмете. Пассивный
// порог у держателя (Gate.Passive) сравнивается с вниманием персонажа
// (Rules.PassiveScore), кости при этом нет. attention задаёт, что вернёт двойник
// правил как пассивное внимание.
func passiveGame(attention int) *Game {
	db := store.NewDB()
	db.Locations["n_room"] = store.Location{ID: "n_room"}
	db.Entities["e_desk"] = store.Entity{ID: "e_desk", Kind: store.EntityThing, Node: "n_room"}
	db.Facts["f_tell"] = store.Fact{ID: "f_tell", Key: "tell"}
	db.Holders["f_tell"] = []store.FactHolder{{
		FactID: "f_tell", HolderID: "e_desk",
		Gate: store.Gate{Verbs: []string{"examine"}, Passive: 12},
	}}
	return NewGame(Config{
		DB: db, Rules: fixedRules{out: OutcomeSuccess, passive: attention}, Dice: nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_room", Actor: "pc",
	})
}

// Внимание берёт порог (12 ≥ 12): tell открывается детерминированно, БЕЗ кости.
func TestPassiveTellRevealsWithoutRollWhenAttentionClears(t *testing.T) {
	g := passiveGame(12)
	got := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_desk"}})
	if len(got.Learned) == 0 {
		t.Fatal("пассивный tell не открылся при достаточном внимании")
	}
	if got.Res != nil {
		t.Error("пассивное раскрытие бросило кость — оно должно быть детерминированным")
	}
}

// Внимания не хватает (10 < 12): tell НЕ открывается, кости нет, и это НЕ лок —
// повтор возможен (провал по пассиву не запирает; знание не записано).
func TestPassiveTellStaysHiddenAndRetryableBelowThreshold(t *testing.T) {
	g := passiveGame(10)
	got := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_desk"}})
	if len(got.Learned) != 0 {
		t.Fatal("пассивный tell открылся при недостатке внимания")
	}
	if got.Res != nil {
		t.Error("пустой пассив бросил кость")
	}
	if g.K.Knows("f_tell") {
		t.Error("недобранный пассив записал знание — повтор станет невозможен (это лок на провале)")
	}
}
