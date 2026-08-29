package server

import (
	"testing"

	"github.com/kliuchnikovv/dnd/view"
)

// Успешный move_zone переставляет игрока: ядро резолвит бросок, сервер меняет
// узел (шов afterAction из CLI). Без этого шва ход крутил бы кость впустую.
func TestApplyMoveChangesNode(t *testing.T) {
	m := NewManager(casesRoot)
	id, _ := m.Create("harbour", 1)
	rt, _ := m.Get(id)

	var moveTok string
	var target string
	for _, a := range rt.game.Affordances(rt.spokenTo) {
		if a.Intent.Verb == "move_zone" {
			moveTok = view.OptionToken(a)
			target = string(a.Intent.Args.Node)
		}
	}
	if moveTok == "" {
		t.Fatal("в стартовом наборе нет move_zone")
	}

	before := string(rt.game.Node)
	ok, msg := rt.applyInput(1, inputPayload{Token: moveTok})
	if !ok {
		t.Fatalf("move не применился: %s", msg)
	}
	after := string(rt.game.Node)
	if after == before {
		t.Fatalf("узел не изменился после успешного move_zone: остался %s", before)
	}
	if after != target {
		t.Fatalf("узел %s, ждали цель хода %s", after, target)
	}
}
