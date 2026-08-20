package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// nOfMGame добавляет к базовому фикстуру факт, чей гейт требует любые два из
// трёх наблюдений — ровно та форма, которой не хватало рукописным делам.
func nOfMGame(req *store.Requirement) *Game {
	g := turnGame(OutcomeSuccess)
	for _, id := range []store.FactID{"f_a", "f_b", "f_c", "f_target"} {
		g.DB.Facts[id] = store.Fact{ID: id, Key: string(id)}
	}
	g.DB.Holders["f_target"] = []store.FactHolder{{
		FactID: "f_target", HolderID: "e_toke", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal", Requires: req},
	}}
	return g
}

func askTarget(g *Game) TurnResult {
	return g.Apply(Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_toke", Topic: "f_target"}})
}

func TestGateWithThresholdNeedsNOfM(t *testing.T) {
	g := nOfMGame(store.RequireN(2, "f_a", "f_b", "f_c"))

	if got := askTarget(g); !got.Refused {
		t.Fatal("факт выдан при нуле собранных предпосылок")
	}
	g.K.Learn("f_a", "e_bern")
	if got := askTarget(g); !got.Refused {
		t.Fatal("порог 2 пройден одной предпосылкой")
	}
	g.K.Learn("f_c", "e_bern")
	got := askTarget(g)
	if got.Refused {
		t.Fatalf("порог 2 не пройден двумя предпосылками: %s", got.Refusal)
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_target" {
		t.Errorf("факт не выдан: %v", got.Learned)
	}
}

func TestGateWithNOneBehavesAsOr(t *testing.T) {
	g := nOfMGame(store.RequireN(1, "f_a", "f_b", "f_c"))
	if got := askTarget(g); !got.Refused {
		t.Fatal("факт выдан без единой предпосылки")
	}
	g.K.Learn("f_b", "e_bern") // любая из трёх
	if got := askTarget(g); got.Refused {
		t.Errorf("n=1 не сработал как OR: %s", got.Refusal)
	}
}

func TestGateWithRequireAllStillNeedsEveryFact(t *testing.T) {
	// Регрессия: поведение «нужны все» не изменилось.
	g := nOfMGame(store.RequireAll("f_a", "f_b"))
	g.K.Learn("f_a", "e_bern")
	if got := askTarget(g); !got.Refused {
		t.Fatal("AND-гейт пропустил при одной из двух предпосылок")
	}
	g.K.Learn("f_b", "e_bern")
	if got := askTarget(g); got.Refused {
		t.Errorf("AND-гейт не пропустил при всех предпосылках: %s", got.Refusal)
	}
}

func TestGateWithoutRequiresIsOpen(t *testing.T) {
	g := nOfMGame(nil)
	if got := askTarget(g); got.Refused {
		t.Errorf("гейт без предпосылок отказал: %s", got.Refusal)
	}
}
