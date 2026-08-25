package core

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func placesGame(start ...store.NodeID) *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay",
		Adjacent: []store.NodeID{"n_warehouse", "n_forge"}}
	db.Locations["n_warehouse"] = store.Location{ID: "n_warehouse",
		Adjacent: []store.NodeID{"n_quay"}}
	db.Locations["n_forge"] = store.Location{ID: "n_forge",
		Adjacent: []store.NodeID{"n_quay", "n_tavern"}}
	db.Locations["n_tavern"] = store.Location{ID: "n_tavern",
		Adjacent: []store.NodeID{"n_forge"}}
	db.Facts["f_ledger"] = store.Fact{ID: "f_ledger", Key: "подчистка в гроссбухе"}
	db.Unlocks["f_ledger"] = []store.FactUnlock{
		{FactID: "f_ledger", UnlocksKind: "node", UnlocksID: "n_tavern"},
	}
	return NewGame(Config{DB: db, CaseID: "harbour", Start: "n_quay",
		StartPlaces: start, Rules: fixedRules{OutcomeSuccess}, Dice: nilDice{}})
}

// Место, где игрок стоит, известно всегда: иначе он не знает, где он.
func TestStartNodeIsKnown(t *testing.T) {
	g := placesGame()
	if !g.KnowsPlace("n_quay") {
		t.Error("стартовый узел неизвестен")
	}
	if g.KnowsPlace("n_warehouse") {
		t.Error("смежное место известно само по себе — тогда знание ничего не гейтит")
	}
}

// Автор вправе объявить место известным с начала: брифинг «Гавани» называет
// склад первой строкой, и не знать о нём игрок не может.
func TestStartPlacesAreKnown(t *testing.T) {
	g := placesGame("n_warehouse")
	if !g.KnowsPlace("n_warehouse") {
		t.Error("объявленное автором место неизвестно")
	}
}

// fact_unlocks вида node в новом смысле: узнал факт — узнал о месте.
func TestLearnedFactMakesPlaceKnown(t *testing.T) {
	g := placesGame()
	if g.KnowsPlace("n_tavern") {
		t.Fatal("место известно до факта")
	}
	g.applyUnlocksFor("f_ledger")
	if !g.KnowsPlace("n_tavern") {
		t.Error("факт не открыл место")
	}
}

// Повтор возвращает «уже знали»: на этом стоит идемпотентность мутации.
func TestKnowPlaceReportsFirstTimeOnly(t *testing.T) {
	g := placesGame()
	if !g.knowPlace("n_forge") {
		t.Error("первый рассказ не считается новым")
	}
	if g.knowPlace("n_forge") {
		t.Error("повтор объявлен новым")
	}
}

// Идти можно в любое известное место, смежное или нет: посёлок маленький, ноги
// есть. Граф остаётся для прозы дороги и для того, о чём вправе рассказать
// персонаж, — но стеной быть перестаёт.
func TestMoveGoesToAnyKnownPlace(t *testing.T) {
	g := placesGame()
	g.applyUnlocksFor("f_ledger") // таверна известна, но не смежна пристани

	got := g.Apply(Intent{Verb: "move_zone", Args: Args{Node: "n_tavern"}})
	if got.Refused {
		t.Errorf("несмежное известное место отвергнуто: %s", got.Refusal)
	}
}

// Неизвестное место недоступно незнанием, а не запретом. Отказ не подсказывает,
// чем открыть, — как и все отказы гейтов.
func TestMoveToUnknownPlaceIsRefused(t *testing.T) {
	g := placesGame()
	got := g.Apply(Intent{Verb: "move_zone", Args: Args{Node: "n_warehouse"}})
	if !got.Refused {
		t.Fatal("неизвестное место пропущено")
	}
	for _, leak := range []string{"f_ledger", "узна", "спрос"} {
		if strings.Contains(got.Refusal, leak) {
			t.Errorf("отказ подсказывает, чем открыть: %q", got.Refusal)
		}
	}
}

// Список доступного — известные места дела без того, где игрок стоит.
func TestReachableIsKnownPlacesWithoutCurrent(t *testing.T) {
	g := placesGame("n_warehouse")
	got := g.ReachableNodes()
	if len(got) != 1 || got[0] != "n_warehouse" {
		t.Errorf("список доступного = %v, ожидалось [n_warehouse]", got)
	}
}
