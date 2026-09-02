package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func unlockGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Adjacent: []store.NodeID{"n_cellar"}}
	db.Locations["n_cellar"] = store.Location{ID: "n_cellar", Adjacent: []store.NodeID{"n_quay"}}
	db.Entities["e_toke"] = store.Entity{ID: "e_toke", Kind: store.EntityNPC, Node: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_key"] = store.Fact{ID: "f_key"}
	db.Facts["f_deep"] = store.Fact{ID: "f_deep"}
	// Спящий держатель: факт у Токе есть, но темы нет, пока не открыт f_key.
	db.Holders["f_deep"] = []store.FactHolder{{
		FactID: "f_deep", HolderID: "e_toke", Mandatory: true, Latent: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}
	db.Unlocks["f_key"] = []store.FactUnlock{
		{FactID: "f_key", UnlocksKind: "topic", UnlocksID: "f_deep"},
		{FactID: "f_key", UnlocksKind: "node", UnlocksID: "n_cellar"},
	}
	db.Clocks["c"] = &store.Clock{ID: "c", Segments: 6, TickPolicy: "on_cost"}
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

// Latent-держатель до разблокировки для holderFor не существует — то же
// самое "нет держателя", что и для незнающего собеседника, и по новому
// правилу это больше не отказ: вопрос уходит в бросок вхолостую, факт не
// выдаётся.
func TestLatentHolderIsInvisibleUntilUnlocked(t *testing.T) {
	g := unlockGame()
	g.K.Learn("f_deep", "e_bern") // тема формально в банке
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_deep"}})
	if got.Refused {
		t.Fatalf("спящий держатель отказал вместо молчания: %s", got.Refusal)
	}
	if len(got.Learned) != 0 {
		t.Errorf("спящий держатель отдал факт до разблокировки: %+v", got.Learned)
	}
}

// Факт открывает тему через unlocked и место через known_places: два разных
// хранилища, потому что тема — про дело, а место — про географию.
func TestLearningUnlocksTopicAndOpensPlace(t *testing.T) {
	g := unlockGame()
	if g.KnowsPlace("n_cellar") {
		t.Fatal("место известно до факта")
	}
	g.K.Learn("f_key", "e_toke")
	g.applyUnlocksFor("f_key")
	if !g.Unlocked("topic", "f_deep") {
		t.Error("тема не открыта")
	}
	if !g.KnowsPlace("n_cellar") {
		t.Error("место не стало известным")
	}
	g.K.Learn("f_deep", "e_bern")
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_deep"}})
	if got.Refused {
		t.Errorf("спящий держатель молчит после разблокировки: %s", got.Refusal)
	}
}

func TestUnlocksFireFromApply(t *testing.T) {
	// Разблокировка должна происходить сама, а не по отдельному вызову.
	g := unlockGame()
	g.DB.Holders["f_key"] = []store.FactHolder{{
		FactID: "f_key", HolderID: "e_toke", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}
	g.K.Learn("f_key", "e_bern") // тема в банке
	g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_key"}})
	if !g.KnowsPlace("n_cellar") {
		t.Error("Apply не применил разблокировки выученного факта")
	}
}

// Список доступного растёт от знания, а не от смежности: до факта место в
// нём отсутствует, после факта появляется — независимо от того, что n_cellar
// и так смежен старту по графу (это проверяет TestMoveGoesToAnyKnownPlace).
func TestReachableNodesRespectKnownPlaces(t *testing.T) {
	g := unlockGame()
	if got := g.ReachableNodes(); len(got) != 0 {
		t.Errorf("неизвестный узел объявлен достижимым: %v", got)
	}
	g.K.Learn("f_key", "e_toke")
	g.applyUnlocksFor("f_key")
	got := g.ReachableNodes()
	if len(got) != 1 || got[0] != "n_cellar" {
		t.Errorf("достижимые узлы = %v, ожидалось [n_cellar]", got)
	}
}
