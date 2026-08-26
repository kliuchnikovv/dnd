package core

import (
	"reflect"
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// affordGame — узел, где есть всё сразу: человек, известная тема, три детали,
// известное место и носимая бумага. Набор обязан выбирать из этого, а не
// перечислять всё.
func affordGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Name: "Пристань"}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Name: "Кузница"}
	db.Entities["e_bern"] = store.Entity{ID: "e_bern", Name: "Берн, стражник",
		Kind: store.EntityNPC, Node: "n_quay"}
	db.Entities["e_body"] = store.Entity{ID: "e_body", Name: "Тело Халдена",
		Kind: store.EntityThing, Node: "n_quay"}
	db.Props["n_quay"] = []store.SceneProp{
		{ID: "p_nets", Node: "n_quay", Name: "Ворох сетей", Kind: "clutter"},
		{ID: "p_crates", Node: "n_quay", Name: "Ящики у стены", Kind: "furniture"},
	}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_known"] = store.Fact{ID: "f_known", Key: "тело нашли на складе"}
	db.Facts["f_secret"] = store.Fact{ID: "f_secret", Key: "гильдейская печать на шнуре"}
	db.Holders["f_secret"] = []store.FactHolder{{
		FactID: "f_secret", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "hard"},
	}}
	db.Items["i_writ"] = store.Item{ID: "i_writ", Name: "Предписание магистрата", Kind: "writ"}
	g := NewGame(Config{
		DB: db, Rules: fixedRules{OutcomeSuccess}, Dice: nilDice{},
		Truth:   accusation.NewTruth("bern", "cord", "night", "debt"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
		StartPlaces: []store.NodeID{"n_forge"},
	})
	g.K.Learn("f_known", "e_briefing")
	g.Acquire("i_writ")
	return g
}

// Набор детерминирован: он часть той же правды, что (seed, script). Плавающий
// порядок сделал бы прогон невоспроизводимым, а игроку — меню, которое
// перетасовывается само.
func TestAffordancesAreDeterministic(t *testing.T) {
	g := affordGame()
	first := g.Affordances()
	second := g.Affordances()
	if !reflect.DeepEqual(first, second) {
		t.Errorf("два вызова разошлись:\n%+v\n%+v", first, second)
	}
	if len(first) < 2 || len(first) > 4 {
		t.Errorf("в наборе %d вариантов, ожидалось 2–4: %+v", len(first), first)
	}
}

// Ни один вариант не называет того, чего парти не знает. Утечка здесь была бы
// не в формулировке, а в САМОМ НАБОРЕ: список, где одна опция ведёт к разгадке,
// спойлерит безупречными словами (ADR-0003, T2).
func TestAffordancesNameNothingUnknown(t *testing.T) {
	g := affordGame()
	for _, a := range g.Affordances() {
		if topic := a.Intent.Args.Topic; topic != "" && !g.K.Knows(topic) {
			t.Errorf("в набор попала неизвестная тема %q", topic)
		}
		if node := a.Intent.Args.Node; node != "" && !g.KnowsPlace(node) {
			t.Errorf("в набор попало неизвестное место %q", node)
		}
		if item := a.Intent.Args.Item; item != "" && !g.Carries(store.ItemID(item)) {
			t.Errorf("в набор попал ненесомый предмет %q", item)
		}
	}
}

// Цели осмотра берутся из деталей узла в порядке ID и держателями не
// отбираются. Иначе набор разметил бы, где лежит авторский контент, — та же
// карта решения, которую запрещает камуфляжный инвариант.
func TestExamineTargetsIgnoreHolders(t *testing.T) {
	g := affordGame()
	plain := examineTargetsOf(g.Affordances())

	// У сетей появляется держатель — набор от этого меняться не должен.
	g.DB.Holders["f_secret"] = append(g.DB.Holders["f_secret"], store.FactHolder{
		FactID: "f_secret", HolderID: "p_nets", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"search"}, Threshold: "easy"},
	})
	if got := examineTargetsOf(g.Affordances()); !reflect.DeepEqual(plain, got) {
		t.Errorf("набор пошёл за держателями: %v → %v", plain, got)
	}
}

func examineTargetsOf(list []Affordance) []store.EntityID {
	var out []store.EntityID
	for _, a := range list {
		if a.Intent.Verb == "examine" {
			out = append(out, a.Intent.Args.Target)
		}
	}
	return out
}

// Тег проверки — класс глагола из реестра, и только там, где бросок есть.
// Порога в теге нет намеренно: число живёт в гейте держателя, то есть в данных
// дела, и напечатать его значит разметить авторские цели.
func TestCheckTagComesFromTheVerbRegistry(t *testing.T) {
	for _, a := range affordGame().Affordances() {
		def, ok := Verbs[a.Intent.Verb]
		if !ok {
			t.Fatalf("вариант несёт глагол вне реестра: %q", a.Intent.Verb)
		}
		switch {
		case rollDecidedElsewhere[a.Intent.Verb]:
			if a.Check != "" {
				t.Errorf("%s: бросок решает не реестр, а тег обещан: %q",
					a.Intent.Verb, a.Check)
			}
		case def.Rolls && a.Check != def.Class:
			t.Errorf("%s: тег %q, класс реестра %q", a.Intent.Verb, a.Check, def.Class)
		case !def.Rolls && a.Check != "":
			t.Errorf("%s броска не требует, а тег есть: %q", a.Intent.Verb, a.Check)
		}
	}
}

// Каждый вариант обязан быть исполнимым: предложить ход, который ядро отклонит,
// значит соврать игроку меню.
func TestEveryAffordanceIsAccepted(t *testing.T) {
	g := affordGame()
	for _, a := range g.Affordances() {
		if r := g.Check(a.Intent); r.Refused {
			t.Errorf("%s → %q отклонён ядром: %s",
				a.Intent.Verb, a.Intent.Args.Target, r.Refusal)
		}
	}
}

// Бедный узел даёт меньше вариантов, а не выдуманные. Пустой набор — законный
// исход: свободный ввод рядом и равноправен.
func TestPoorNodeGivesFewerAffordances(t *testing.T) {
	db := store.NewDB()
	db.Locations["n_empty"] = store.Location{ID: "n_empty", Name: "Пустырь"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	g := NewGame(Config{
		DB: db, Rules: fixedRules{OutcomeSuccess}, Dice: nilDice{},
		Truth:   accusation.NewTruth("a", "b", "c", "d"),
		Flavour: map[string]string{}, Start: "n_empty", Actor: "pc",
	})
	if got := g.Affordances(); len(got) != 0 {
		t.Errorf("на пустом узле набор непуст: %+v", got)
	}
}
