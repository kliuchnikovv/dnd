package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func companionGame(t *testing.T) *Game {
	t.Helper()
	g := turnGame(OutcomeFail)
	g.DB.Entities["e_nils"] = store.Entity{
		ID: "e_nils", Kind: store.EntityNPC, Name: "Нильс", Node: "n_quay",
	}
	g.Companion = "e_nils"
	g.Hints = map[store.FactID]string{
		"f_open": "«А вы Токе спрашивали? Он у ворот весь день стоит».",
	}
	return g
}

// Напарник молчит, пока расследование движется: подсказка на каждом ходу — это
// не помощь, а чтение вслух решения.
func TestCompanionIsSilentWhileFactsKeepComing(t *testing.T) {
	g := companionGame(t)
	if _, ok := g.Hint(); ok {
		t.Error("напарник заговорил на первом ходу")
	}
}

// Подсказка приходит после нескольких ходов вхолостую и указывает на цель, а
// не на ответ.
func TestCompanionSpeaksAfterSeveralEmptyTurns(t *testing.T) {
	g := companionGame(t)
	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_toke"}})
	}
	line, ok := g.Hint()
	if !ok {
		t.Fatal("напарник промолчал после трёх пустых ходов")
	}
	if line != g.Hints["f_open"] {
		t.Errorf("реплика не из дела: %q", line)
	}
}

// Найденный факт обнуляет счётчик: игрок, который движется, подсказки не
// получает.
func TestLearningResetsTheCompanion(t *testing.T) {
	g := companionGame(t)
	for i := 0; i < HintAfter; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_toke"}})
	}
	g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if _, ok := g.Hint(); ok {
		t.Error("напарник подсказывает после находки")
	}
}

// Про известное не подсказывают.
func TestCompanionDoesNotPointAtKnownFacts(t *testing.T) {
	g := companionGame(t)
	g.K.Learn("f_open", "e_toke")
	for i := 0; i < HintAfter+2; i++ {
		g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_toke"}})
	}
	if line, ok := g.Hint(); ok {
		t.Errorf("напарник подсказал уже известное: %q", line)
	}
}

// Подсказка звучит один раз: повторять её каждый ход — это уже не напарник.
func TestHintIsSaidOnce(t *testing.T) {
	g := companionGame(t)
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
// «здесь об этом не расскажут», напарник за три прогона не сказал ни слова, и
// игрок решил, что механика ему недоступна.
func TestRefusedProbesCountAsBeingStuck(t *testing.T) {
	g := companionGame(t)
	// Тема не у этого держателя: мир отказывает, ход не тратится.
	stuck := Intent{Verb: "question", Actor: g.Actor,
		Args: Args{Target: "e_nils", Topic: "f_open"}}

	for i := 0; i < HintAfter; i++ {
		if res := g.Apply(stuck); !res.Refused {
			t.Fatalf("проба %d не была отказом: %+v", i, res)
		}
	}
	if _, ok := g.Hint(); !ok {
		t.Error("после трёх отказов подряд напарник молчит")
	}
}

// Отказ ход не тратит — это остаётся верным: часы от него не тикают, и
// счётчик холостых ходов на это не влияет.
func TestRefusalStillCostsNoTurn(t *testing.T) {
	g := companionGame(t)
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
	g := companionGame(t)
	for i := 0; i < HintAfter+2; i++ {
		g.Apply(Intent{Verb: "talk_to", Actor: g.Actor, Args: Args{Target: "e_нет_такого"}})
	}
	if _, ok := g.Hint(); ok {
		t.Error("напарник заговорил от отказов по мягким глаголам")
	}
}
