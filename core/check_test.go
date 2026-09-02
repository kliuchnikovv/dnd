package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// checkCases — вводы, которые мир не принимает. Каждый обязан отказываться
// одинаково и через Check, и через Apply: два места правды о том, что можно,
// разъедутся молча, и чат-режим начнёт показывать реплику Мастера на
// действие, которое ядро не пропустит.
func checkCases() []struct {
	name string
	in   Intent
} {
	return []struct {
		name string
		in   Intent
	}{
		{"неизвестный глагол", Intent{Verb: "танцевать"}},
		{"пустая гипотеза", Intent{Verb: "theorize"}},
		{"сущности нет в деле", Intent{Verb: "question", Args: Args{Target: "e_никого"}}},
		{"сущность в другой локации", Intent{Verb: "question", Args: Args{Target: "e_elsewhere"}}},
		{"туда не пройти", Intent{Verb: "move_zone", Args: Args{Node: "n_далеко"}}},
		{"факт неизвестен", Intent{Verb: "compare", Args: Args{Facts: []store.FactID{"f_нет"}}}},
		{"тема неизвестна", Intent{Verb: "question",
			Args: Args{Target: "e_toke", Topic: "f_нет"}}},
	}
}

func TestCheckRefusesWhatApplyRefuses(t *testing.T) {
	for _, c := range checkCases() {
		g := testGame()
		g.DB.Entities["e_elsewhere"] = store.Entity{
			ID: "e_elsewhere", Kind: store.EntityNPC, Node: "n_другой"}

		want := g.Apply(c.in)
		if !want.Refused {
			t.Fatalf("%s: Apply не отказал — тест проверяет не то, что думает", c.name)
		}

		g2 := testGame()
		g2.DB.Entities["e_elsewhere"] = store.Entity{
			ID: "e_elsewhere", Kind: store.EntityNPC, Node: "n_другой"}
		got := g2.Check(c.in)
		if !got.Refused {
			t.Errorf("%s: Check пропустил то, что Apply отказал", c.name)
			continue
		}
		if got.Refusal != want.Refusal {
			t.Errorf("%s: Check отказал иначе:\n Check: %q\n Apply: %q",
				c.name, got.Refusal, want.Refusal)
		}
	}
}

// Check ничего не меняет — это весь его контракт. Он спрашивается ДО того, как
// игрок увидит ответ Мастера, и если он тикает часами или считает холостые
// ходы, то ход тратится дважды.
func TestCheckChangesNothing(t *testing.T) {
	probes := append(checkCases(), struct {
		name string
		in   Intent
	}{"разрешённое действие", Intent{Verb: "question", Args: Args{Target: "e_toke"}}})

	for _, c := range probes {
		g := testGame()
		before := snapshotAll(g)
		g.Check(c.in)
		if after := snapshotAll(g); after != before {
			t.Errorf("%s: Check изменил состояние:\n до: %+v\nпосле: %+v", c.name, before, after)
		}
	}
}

// Разрешённое действие Check пропускает: иначе чат-режим отказывал бы на всём.
func TestCheckPassesWhatApplyAccepts(t *testing.T) {
	g := testGame()
	if r := g.Check(Intent{Verb: "question", Args: Args{Target: "e_toke"}}); r.Refused {
		t.Errorf("Check отказал законному ходу: %q", r.Refusal)
	}
}

// worldState — то, что ход обязан был бы задеть: часы, знания, расположение,
// счётчик холостых ходов, попытки, гипотезы, узел.
type worldState struct {
	node      store.NodeID
	attempts  int
	dry       int
	theories  int
	grit      int
	harm      int
	clock     int
	knows     int
	disposTok int
}

func snapshotAll(g *Game) worldState {
	return worldState{
		node:      g.Node,
		attempts:  g.Attempts,
		dry:       g.dry,
		theories:  len(g.Theories),
		grit:      g.DB.Characters["pc"].Grit,
		harm:      g.DB.Characters["pc"].Harm,
		clock:     g.DB.Clocks["c_suspicion"].Filled,
		knows:     len(g.K.TopicBank()),
		disposTok: g.D.Disposition("e_toke"),
	}
}
