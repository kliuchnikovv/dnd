package cli

import (
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
)

func proseGame(t *testing.T) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	return core.NewGame(*cfg)
}

// Брифинг не гвардится состоянием: State пуст намеренно, Frame — авторский текст.
func TestBriefingProseHasNoState(t *testing.T) {
	g := proseGame(t)
	p := BriefingProse(g)
	if p.Kind != ProseBriefing {
		t.Fatalf("вид %q", p.Kind)
	}
	if len(p.State) != 0 {
		t.Fatalf("у брифинга появился state: %v", p.State)
	}
}

// Проза места несёт рамку осмотра и дайджест состояния — те же, что печатает
// Render.Scene.
func TestPlaceProseCarriesFrameAndState(t *testing.T) {
	g := proseGame(t)
	p := PlaceProse(g)
	if p.Kind != ProsePlace {
		t.Fatalf("вид %q", p.Kind)
	}
	if p.Frame != g.Flavour("look."+string(g.Node)) {
		t.Fatalf("рамка места разошлась с Flavour")
	}
	if len(p.State) == 0 {
		t.Fatalf("проза места без дайджеста — гвардить нечем")
	}
}

// Отказанный ход прозы исхода не даёт: описывать нечего.
func TestOutcomeProseSkipsRefused(t *testing.T) {
	g := proseGame(t)
	if _, ok := OutcomeProse(g, core.Intent{}, core.TurnResult{Refused: true}); ok {
		t.Fatal("у отказанного хода не должно быть прозы исхода")
	}
	if _, ok := OutcomeProse(g, core.Intent{}, core.TurnResult{}); ok {
		t.Fatal("без FlavourKey прозы исхода нет")
	}
}

// Реплика NPC: разговорный ход с говорящим и авторской рамкой даёт ProseReply
// с именем говорящего; без говорящего или без рамки — не даёт (нечего/некому).
func TestReplyProse(t *testing.T) {
	g := proseGame(t)
	talk := core.Intent{Args: core.Args{Target: "e_bern"}}

	p, ok := ReplyProse(g, talk, core.TurnResult{FlavourKey: "talk.e_bern"})
	if !ok {
		t.Fatal("у разговорного хода с говорящим должна быть реплика")
	}
	if p.Kind != ProseReply {
		t.Fatalf("вид %q, ждали reply", p.Kind)
	}
	if p.Speaking != "Берн, стражник" {
		t.Fatalf("говорящий %q", p.Speaking)
	}

	if _, ok := ReplyProse(g, core.Intent{}, core.TurnResult{FlavourKey: "talk.e_bern"}); ok {
		t.Fatal("без цели-NPC говорить некому — реплики нет")
	}
	if _, ok := ReplyProse(g, talk, core.TurnResult{}); ok {
		t.Fatal("без FlavourKey реплику не на чем заземлить")
	}
	if _, ok := ReplyProse(g, talk, core.TurnResult{FlavourKey: "talk.e_bern", Refused: true}); ok {
		t.Fatal("у отказанного хода реплики нет")
	}
}
