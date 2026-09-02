package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func flavourGame(texts map[string]string) *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Adjacent: []store.NodeID{"n_quay"}}
	db.Entities["e_bern"] = store.Entity{ID: "e_bern", Kind: store.EntityNPC, Node: "n_quay"}
	db.CharactersMap()["pc"] = &store.Character{ID: "pc", Grit: 3}
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth:   accusation.NewTruth("a", "b", "c", "d"),
		Flavour: texts, Start: "n_quay", Actor: "pc",
	})
}

// Общий ключ на глагол избавляет автора дела от необходимости писать текст
// на каждую пару глагол-сущность. Именно этого не хватало soft-глаголам.
func TestFlavourFallsBackToVerb(t *testing.T) {
	g := flavourGame(map[string]string{"talk_to": "Разговор ни о чём."})
	in := Intent{Verb: "talk_to", Args: Args{Target: "e_bern"}}
	if got := g.flavourKey(in); got != "talk_to" {
		t.Errorf("ключ %q, ожидался общий talk_to", got)
	}
}

func TestFlavourPrefersSpecificKey(t *testing.T) {
	g := flavourGame(map[string]string{
		"talk_to":        "общий",
		"talk_to.e_bern": "частный",
	})
	in := Intent{Verb: "talk_to", Args: Args{Target: "e_bern"}}
	if got := g.flavourKey(in); got != "talk_to.e_bern" {
		t.Errorf("ключ %q, ожидался частный talk_to.e_bern", got)
	}
}

// Когда нет ни одного текста, в выводе должен появиться самый частный ключ:
// именно его автору дела и надо написать.
func TestFlavourReportsMostSpecificMissingKey(t *testing.T) {
	g := flavourGame(map[string]string{})
	in := Intent{Verb: "talk_to", Args: Args{Target: "e_bern"}}
	if got := g.flavourKey(in); got != "talk_to.e_bern" {
		t.Errorf("ключ %q, ожидался talk_to.e_bern", got)
	}
}

// Раньше глагол с узлом и без цели давал verb.<текущий узел>, из-за чего
// перемещение подписывалось текстом места, откуда уходят.
func TestFlavourForNodeVerbPrefersDestination(t *testing.T) {
	g := flavourGame(map[string]string{
		"move_zone.n_forge": "в кузницу",
		"move_zone.n_quay":  "с пристани",
	})
	in := Intent{Verb: "move_zone", Args: Args{Node: "n_forge"}}
	if got := g.flavourKey(in); got != "move_zone.n_forge" {
		t.Errorf("ключ %q, ожидался move_zone.n_forge", got)
	}
}

func TestFlavourDeliveryFallsBackFromPairToVerb(t *testing.T) {
	g := flavourGame(map[string]string{"question": "он отвечает нехотя"})
	holder := store.FactHolder{FactID: "f_x", HolderID: "e_bern"}
	in := Intent{Verb: "question", Args: Args{Target: "e_bern", Topic: "f_x"}}
	if got := g.flavourKeyFor(in, holder, true); got != "question" {
		t.Errorf("ключ %q, ожидался общий question", got)
	}
}
