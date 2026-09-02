package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func compareGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Tags: []string{"dark", "rain"}}
	db.CharactersMap()["pc"] = &store.Character{ID: "pc", Grit: 3}
	for _, id := range []store.FactID{"f_alibi", "f_seen_at_quay", "f_lie", "f_unrelated"} {
		db.Facts[id] = store.Fact{ID: id, Key: string(id)}
	}
	db.Contradictions = append(db.Contradictions, store.Contradiction{
		A: "f_alibi", B: "f_seen_at_quay", Reveals: "f_lie", FlavourKey: "compare.alibi_quay",
	})
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{"compare.alibi_quay": "Одно из двух — ложь."},
		Start:   "n_quay", Actor: "pc",
	})
}

func TestCompareNeedsBothFactsKnown(t *testing.T) {
	g := compareGame()
	g.K.Learn("f_alibi", "e_ivar")
	got := g.Compare("f_alibi", "f_seen_at_quay")
	if !got.Refused {
		t.Error("сопоставление прошло с одним известным фактом из двух")
	}
}

func TestCompareFindsContradictionWithoutRoll(t *testing.T) {
	g := compareGame()
	g.K.Learn("f_alibi", "e_ivar")
	g.K.Learn("f_seen_at_quay", "e_nils")
	got := g.Compare("f_alibi", "f_seen_at_quay")
	if got.Refused {
		t.Fatalf("сопоставление отвергнуто: %s", got.Refusal)
	}
	if got.Res != nil {
		t.Error("compare бросил кость — противоречие это свойство данных, не удача")
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_lie" {
		t.Fatalf("противоречие не открыло факт: %v", got.Learned)
	}
	if got.FlavourKey != "compare.alibi_quay" {
		t.Errorf("ключ флейвора = %q", got.FlavourKey)
	}
}

func TestCompareIsOrderIndependent(t *testing.T) {
	g := compareGame()
	g.K.Learn("f_alibi", "e_ivar")
	g.K.Learn("f_seen_at_quay", "e_nils")
	got := g.Compare("f_seen_at_quay", "f_alibi") // обратный порядок
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_lie" {
		t.Errorf("порядок аргументов изменил результат: %v", got.Learned)
	}
}

func TestCompareOfUnrelatedFactsCostsNothing(t *testing.T) {
	g := compareGame()
	g.K.Learn("f_alibi", "e_ivar")
	g.K.Learn("f_unrelated", "e_bern")
	got := g.Compare("f_alibi", "f_unrelated")
	if got.Refused {
		t.Fatal("сопоставление известных фактов отвергнуто")
	}
	if len(got.Learned) != 0 {
		t.Errorf("несвязанные факты дали открытие: %v", got.Learned)
	}
	if len(got.Costs) != 0 {
		t.Errorf("пустое сопоставление стоило %v", got.Costs)
	}
}

func TestNodeTagsComeFromCase(t *testing.T) {
	g := compareGame()
	view := g.SceneView(Intent{Verb: "look"})
	if !view.HasTag("dark") || !view.HasTag("rain") {
		t.Errorf("теги локации не доехали до SceneView: %v", view.NodeTags)
	}
}
