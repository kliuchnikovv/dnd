package adventure

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// sectionKinds — виды секций панели, в порядке появления.
func sectionKinds(p core.Panel) []string {
	out := make([]string, 0, len(p.Sections))
	for _, s := range p.Sections {
		out = append(out, s.Kind)
	}
	return out
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// newAdventurePanelGame — минимальная игра-приключение с узлом, соседом,
// предметом в инвентаре и живым героем-сущностью.
func newAdventurePanelGame(t *testing.T) *core.Game {
	t.Helper()
	db := store.NewDB()
	db.Locations["n_room"] = store.Location{ID: "n_room", Name: "комната", Adjacent: []store.NodeID{"n_hall"}}
	db.Locations["n_hall"] = store.Location{ID: "n_hall", Name: "коридор", Adjacent: []store.NodeID{"n_room"}}
	db.Items["i_torch"] = store.Item{ID: "i_torch", Name: "факел"}
	db.AddItem("party", "i_torch")

	g := core.NewGame(core.Config{DB: db, Scenario: &scenario{}, Start: "n_room", Actor: "chr_kay"})
	db.Entities[store.EntityID(g.Actor)] = store.Entity{ID: store.EntityID(g.Actor), Node: "n_room", HP: 8, MaxHP: 12}
	return g
}

func TestAdventurePanelHasHealthMapInventory(t *testing.T) {
	g := newAdventurePanelGame(t)
	p := (&scenario{}).Panel(g)
	kinds := sectionKinds(p)
	for _, want := range []string{"health", "map", "inventory"} {
		if !containsStr(kinds, want) {
			t.Errorf("нет секции %q: %v", want, kinds)
		}
	}
	if containsStr(kinds, "initiative") {
		t.Errorf("вне боя секции initiative быть не должно: %v", kinds)
	}
}

func TestAdventurePanelHasInitiativeInCombat(t *testing.T) {
	g := newAdventurePanelGame(t)
	db := g.DB
	db.Entities["e_orc"] = store.Entity{ID: "e_orc", Name: "орк", Node: "n_room", HP: 5, MaxHP: 5}
	g.Encounter = &core.Encounter{
		Order: []store.EntityID{store.EntityID(g.Actor), "e_orc"},
		Turn:  0,
		Round: 1,
	}

	p := (&scenario{}).Panel(g)
	if !containsStr(sectionKinds(p), "initiative") {
		t.Fatal("в бою секции initiative нет")
	}
}
