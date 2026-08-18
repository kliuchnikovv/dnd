package core

import (
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
		Truth: accusation.NewTruth("toke", "cord", "night", "audit"),
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
