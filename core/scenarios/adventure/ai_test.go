package adventure

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

func TestNPCTurnAttacksWhenSameNode(t *testing.T) {
	db := store.NewDB()
	db.Locations["n_room"] = store.Location{ID: "n_room"}
	db.Entities["e_orc"] = store.Entity{ID: "e_orc", Node: "n_room", HP: 5, MaxHP: 5}
	g := core.NewGame(core.Config{DB: db, Scenario: &scenario{}, Start: "n_room", Actor: "chr_kay"})
	db.Entities[store.EntityID(g.Actor)] = store.Entity{ID: store.EntityID(g.Actor), Node: "n_room", HP: 10, MaxHP: 10}

	in, ok := npcTurn(g, "e_orc")
	if !ok {
		t.Fatal("ожидался ход")
	}
	if in.Verb != "attack" {
		t.Fatalf("ожидался attack, получили %q", in.Verb)
	}
	if in.Args.Target != store.EntityID(g.Actor) {
		t.Fatalf("цель атаки не игрок: %q", in.Args.Target)
	}
}

func TestNPCTurnMovesTowardWhenElsewhere(t *testing.T) {
	db := store.NewDB()
	db.Locations["n_far"] = store.Location{ID: "n_far", Adjacent: []store.NodeID{"n_mid"}}
	db.Locations["n_mid"] = store.Location{ID: "n_mid", Adjacent: []store.NodeID{"n_far", "n_room"}}
	db.Locations["n_room"] = store.Location{ID: "n_room", Adjacent: []store.NodeID{"n_mid"}}
	db.Entities["e_orc"] = store.Entity{ID: "e_orc", Node: "n_far", HP: 5, MaxHP: 5}
	g := core.NewGame(core.Config{DB: db, Scenario: &scenario{}, Start: "n_room", Actor: "chr_kay"})
	db.Entities[store.EntityID(g.Actor)] = store.Entity{ID: store.EntityID(g.Actor), Node: "n_room", HP: 10, MaxHP: 10}

	in, ok := npcTurn(g, "e_orc")
	if !ok {
		t.Fatal("ожидался ход")
	}
	if in.Verb != "move_zone" {
		t.Fatalf("ожидался move_zone, получили %q", in.Verb)
	}
	if in.Args.Node != "n_mid" {
		t.Fatalf("ожидался шаг в n_mid, получили %q", in.Args.Node)
	}
}

func TestStepTowardNoPath(t *testing.T) {
	db := store.NewDB()
	db.Locations["n_a"] = store.Location{ID: "n_a"}
	db.Locations["n_b"] = store.Location{ID: "n_b"}
	g := core.NewGame(core.Config{DB: db, Scenario: &scenario{}, Start: "n_a"})

	if _, ok := stepToward(g, "n_a", "n_b"); ok {
		t.Fatal("без смежности пути быть не должно")
	}
}
