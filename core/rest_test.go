package core

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func restGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 0, Harm: 2}
	db.Clocks["c_tide"] = &store.Clock{ID: "c_tide", Name: "Прилив", Segments: 6, TickPolicy: "on_cost"}
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func TestShortRestRestoresGritOnly(t *testing.T) {
	g := restGame()
	g.Rest(RestShort)
	ch := g.DB.Characters["pc"]
	if ch.Grit != GritMax {
		t.Errorf("grit = %d, ожидалось %d", ch.Grit, GritMax)
	}
	if ch.Harm != 2 {
		t.Errorf("короткий отдых снял ранение: harm = %d", ch.Harm)
	}
	if g.DB.Clocks["c_tide"].Filled != 0 {
		t.Error("короткий отдых тикнул часы")
	}
}

func TestLongRestHealsOneHarmAndTicks(t *testing.T) {
	g := restGame()
	g.Rest(RestLong)
	ch := g.DB.Characters["pc"]
	if ch.Harm != 1 {
		t.Errorf("harm = %d, ожидалось 1", ch.Harm)
	}
	if g.DB.Clocks["c_tide"].Filled != 1 {
		t.Errorf("длинный отдых не тикнул часы: %d", g.DB.Clocks["c_tide"].Filled)
	}
}

func TestRestPreviewNamesTheCostBeforePaying(t *testing.T) {
	g := restGame()
	got := g.RestPreview(RestLong)
	if len(got) != 1 || got[0] != "c_tide" {
		t.Errorf("предпросмотр цены = %v, ожидалось [c_tide]", got)
	}
	if g.DB.Clocks["c_tide"].Filled != 0 {
		t.Error("предпросмотр сам заплатил цену")
	}
	if len(g.RestPreview(RestShort)) != 0 {
		t.Error("короткий отдых объявил цену")
	}
}

func TestLongRestWithoutHarmStillCosts(t *testing.T) {
	// Длинный отдых — структурный переход, а не арифметика: время идёт даже
	// у здорового.
	g := restGame()
	g.DB.Characters["pc"].Harm = 0
	g.Rest(RestLong)
	if g.DB.Clocks["c_tide"].Filled != 1 {
		t.Error("длинный отдых без ранений обошёлся бесплатно")
	}
}

// Третья ячейка ранений выводит из строя: дальше только отдых. Без потолка
// harm рос без предела, а «выведен из строя» не наступало никогда.
func TestThirdHarmCellTakesTheCharacterOut(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Characters["pc"].Harm = HarmMax

	res := g.Apply(Intent{Verb: "question", Args: Args{
		Target: "e_toke", Topic: "f_open",
	}})
	if !res.Refused {
		t.Fatal("выведенный из строя продолжает работать")
	}
	if !strings.Contains(res.Refusal, "из строя") {
		t.Errorf("отказ не объясняет причину: %q", res.Refusal)
	}
}

// Свободная проба и отдых остаются: иначе выход из строя — тупик, а не
// состояние.
func TestIncapacitatedCanStillLookAndRest(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.DB.Characters["pc"].Harm = HarmMax

	if res := g.Apply(Intent{Verb: "look"}); res.Refused {
		t.Errorf("осмотреться нельзя: %s", res.Refusal)
	}
	if res := g.Rest(RestLong); res.Refused {
		t.Errorf("отдохнуть нельзя: %s", res.Refusal)
	}
	if h := g.DB.Characters["pc"].Harm; h != HarmMax-1 {
		t.Errorf("длинный отдых не снял ячейку: harm %d", h)
	}
}

// Ранения не растут выше потолка: четвёртой ячейки не существует.
func TestHarmNeverExceedsItsCap(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	ch := g.DB.Characters["pc"]
	for i := 0; i < 6; i++ {
		g.applyMutations([]Mutation{{Kind: MutHarm, Target: "pc", Delta: 1}})
	}
	if ch.Harm != HarmMax {
		t.Errorf("harm %d при потолке %d", ch.Harm, HarmMax)
	}
}
