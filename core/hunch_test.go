package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func hunchGame(t *testing.T) *Game {
	t.Helper()
	g := turnGame(OutcomeFail)
	g.DB.Entities["e_nils"] = store.Entity{
		ID: "e_nils", Kind: store.EntityNPC, Name: "Нильс", Node: "n_quay",
	}
	g.Hints = map[store.FactID]string{
		"f_open": "Токе стоит у ворот весь день. Его так и не спросили.",
	}
	return g
}

// Чутьё молчит, пока расследование движется: подсказка на каждом ходу — это
// не помощь, а чтение вслух решения.
func TestHunchIsSilentWhileFactsKeepComing(t *testing.T) {
	g := hunchGame(t)
	if _, ok := g.Hint(); ok {
		t.Error("чутьё сработало на первом ходу")
	}
}

// Подсказка приходит после нескольких ходов вхолостую и указывает на цель, а
// не на ответ.
func TestHunchArrivesAfterSeveralEmptyTurns(t *testing.T) {
	g := hunchGame(t)
	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_toke"}})
	}
	line, ok := g.Hint()
	if !ok {
		t.Fatal("чутьё промолчало после трёх пустых ходов")
	}
	if line != g.Hints["f_open"] {
		t.Errorf("подсказка не из дела: %q", line)
	}
}

// Найденный факт обнуляет счётчик: игрок, который движется, подсказки не
// получает.
func TestLearningResetsTheHunch(t *testing.T) {
	g := hunchGame(t)
	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_toke"}})
	}
	g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if _, ok := g.Hint(); ok {
		t.Error("чутьё подсказывает после находки")
	}
}

// Про известное не подсказывают.
func TestHunchDoesNotPointAtKnownFacts(t *testing.T) {
	g := hunchGame(t)
	g.K.Learn("f_open", "e_toke")
	for i := 0; i < HintAfter+2; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_toke"}})
	}
	if line, ok := g.Hint(); ok {
		t.Errorf("чутьё подсказало уже известное: %q", line)
	}
}

// Подсказка приходит один раз: повторять её каждый ход — это чтение решения вслух.
func TestHintIsSaidOnce(t *testing.T) {
	g := hunchGame(t)
	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_toke"}})
	}
	if _, ok := g.Hint(); !ok {
		t.Fatal("подсказки не было")
	}
	if _, ok := g.Hint(); ok {
		t.Error("подсказка повторилась подряд")
	}
}

// Отказ — самый чистый признак того, что игрок встал: он попробовал, и мир не
// принял. Живой плейтест уткнулся ровно в это: агент произвёл десятки отказов
// «здесь об этом не расскажут», подсказка за три прогона не пришла ни разу, и
// игрок решил, что механика ему недоступна.
func TestRefusedProbesCountAsBeingStuck(t *testing.T) {
	g := hunchGame(t)
	// Тема не у этого держателя: мир отказывает, ход не тратится.
	stuck := Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_nils", Topic: "f_open"}}

	for i := 0; i < HintAfter; i++ {
		if res := g.Apply(stuck); !res.Refused {
			t.Fatalf("проба %d не была отказом: %+v", i, res)
		}
	}
	if _, ok := g.Hint(); !ok {
		t.Error("после трёх отказов подряд чутьё молчит")
	}
}

// Отказ ход не тратит — это остаётся верным: часы от него не тикают, и
// счётчик холостых ходов на это не влияет.
func TestRefusalStillCostsNoTurn(t *testing.T) {
	g := hunchGame(t)
	before := g.C.Snapshot()
	g.Apply(Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_nils", Topic: "f_open"}})
	for i, c := range g.C.Snapshot() {
		if c.Filled != before[i].Filled {
			t.Errorf("отказ продвинул часы %s: было %d, стало %d",
				c.ID, before[i].Filled, c.Filled)
		}
	}
}

// Мягкий глагол «застрял» не означает: осмотреться и поговорить — не пробы, и
// отказ по ним счётчик не двигает.
func TestRefusedSoftVerbsDoNotCount(t *testing.T) {
	g := hunchGame(t)
	for i := 0; i < HintAfter+2; i++ {
		g.Apply(Intent{Verb: "talk_to", Actor: g.Actor, Args: Args{Target: "e_нет_такого"}})
	}
	if _, ok := g.Hint(); ok {
		t.Error("чутьё сработало от отказов по мягким глаголам")
	}
}

// Подсказке не нужен говорящий. Раньше её произносил напарник — и отвечал за
// слова, которые выбрал движок: подсказка называет человека или запись (в
// «Гавани» четыре из шести), а это ровно то, что гвард обязан рубить как
// выдумку персонажа. Чутьё принадлежит самому игроку, и авторства ни у кого не
// отнимает.
func TestHunchNeedsNoSpeaker(t *testing.T) {
	g := hunchGame(t)
	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_toke"}})
	}
	if _, ok := g.Hint(); !ok {
		t.Error("подсказка не пришла без объявленного напарника")
	}
}

// Чутьё не знает того, чего игрок не видел. Раньше оно указывало на держателя
// в СМЕЖНОМ узле — то есть «туда можно дойти», а не «ты там был», — и живой
// прогон получил на Пристани подсказку про кузнеца, которого не встречал:
// «Ивар не отходит от стойки, спросите его про деньги». Это оракул, а не
// чутьё: имя и мотив взялись из ниоткуда.
func TestHunchDoesNotPointAtUnvisitedPlaces(t *testing.T) {
	g := hunchGame(t)
	// Держатель подсказки уезжает в соседний узел, где игрок не был.
	g.DB.Locations["n_forge"] = store.Location{ID: "n_forge", Adjacent: []store.NodeID{"n_quay"}}
	loc := g.DB.Locations[g.Node]
	loc.Adjacent = append(loc.Adjacent, "n_forge")
	g.DB.Locations[g.Node] = loc
	e := g.DB.Entities["e_toke"]
	e.Node = "n_forge"
	g.DB.Entities["e_toke"] = e

	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_nils"}})
	}
	if line, ok := g.Hint(); ok {
		t.Errorf("чутьё указало на невиденное место: %q", line)
	}
}

// А посещённое — законно: там игрок был и видел, кто и что там есть.
func TestHunchPointsAtVisitedPlaces(t *testing.T) {
	g := hunchGame(t)
	g.DB.Locations["n_forge"] = store.Location{ID: "n_forge", Adjacent: []store.NodeID{"n_quay"}}
	loc := g.DB.Locations[g.Node]
	loc.Adjacent = append(loc.Adjacent, "n_forge")
	g.DB.Locations[g.Node] = loc
	e := g.DB.Entities["e_toke"]
	e.Node = "n_forge"
	g.DB.Entities["e_toke"] = e

	g.MoveTo("n_forge")
	g.MoveTo("n_quay")
	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_nils"}})
	}
	if _, ok := g.Hint(); !ok {
		t.Error("чутьё молчит про место, где игрок уже был")
	}
}

// Стартовый узел посещён с самого начала: игрок в нём стоит.
func TestStartNodeCountsAsVisited(t *testing.T) {
	g := hunchGame(t)
	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_nils"}})
	}
	if _, ok := g.Hint(); !ok {
		t.Error("чутьё молчит про место, в котором игрок стоит")
	}
}
