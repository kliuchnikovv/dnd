package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
)

// viewJSON — session_state текущего состояния как JSON: эталон сравнения
// «до и после рестарта». Frame.id в него не входит, поэтому счётчики кадров
// двух менеджеров сравнение не портят.
func viewJSON(t *testing.T, rt *sessionRuntime) string {
	t.Helper()
	f := rt.snapshotView()
	return string(f.Payload)
}

// playTurns проигрывает n ходов, выбирая каждый раз первый показанный вариант.
// Детерминизм на seed: тот же набор входов даёт то же состояние.
func playTurns(t *testing.T, rt *sessionRuntime, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		rt.mu.Lock()
		offered := rt.game.Affordances(rt.spokenTo)
		rt.mu.Unlock()
		if len(offered) == 0 {
			t.Fatalf("ход %d: играть нечем", i+1)
		}
		tok := view.OptionToken(offered[0])
		ok, msg := rt.applyInput(context.Background(), i+1, inputPayload{Token: tok})
		if !ok {
			t.Fatalf("ход %d не применился: %s", i+1, msg)
		}
	}
}

// Сессия переживает рестарт: свежий Manager над тем же журналом восстанавливает
// то же состояние (реплей = то же состояние).
func TestSessionSurvivesRestart(t *testing.T) {
	st := NewMemStore()
	m1 := NewManagerWithStore(casesRoot, st)
	id, err := m1.Create("harbour", 1)
	if err != nil {
		t.Fatal(err)
	}
	rt1, _ := m1.Get(id)
	playTurns(t, rt1, 3)

	wantView := viewJSON(t, rt1)
	wantNode := rt1.game.Node
	wantTurn := rt1.turn

	// «Рестарт»: новый Manager, пустой кэш, тот же журнал.
	m2 := NewManagerWithStore(casesRoot, st)
	rt2, ok := m2.Get(id)
	if !ok {
		t.Fatalf("сессия не восстановилась из журнала")
	}
	if rt2.game.Node != wantNode {
		t.Fatalf("узел после рестарта %s, был %s", rt2.game.Node, wantNode)
	}
	if rt2.turn != wantTurn {
		t.Fatalf("номер хода после рестарта %d, был %d", rt2.turn, wantTurn)
	}
	if got := viewJSON(t, rt2); got != wantView {
		t.Fatalf("состояние разошлось после рестарта:\nбыло:  %s\nстало: %s", wantView, got)
	}
}

// Незавершённый хвост (pending после падения между «записал» и «применил»)
// доигрывается при восстановлении и помечается applied.
func TestReconstructAppliesPendingTail(t *testing.T) {
	st := NewMemStore()
	m1 := NewManagerWithStore(casesRoot, st)
	id, _ := m1.Create("harbour", 1)
	rt1, _ := m1.Get(id)

	// Готовим интент move_zone из показанного набора и пишем его как pending
	// напрямую в журнал — имитация падения ДО применения и отметки.
	rt1.mu.Lock()
	var moveIntent core.Intent
	for _, a := range rt1.game.Affordances(rt1.spokenTo) {
		if a.Intent.Verb == "move_zone" {
			moveIntent = a.Intent
			moveIntent.Actor = rt1.game.Actor
		}
	}
	rt1.mu.Unlock()
	if moveIntent.Verb == "" {
		t.Fatal("нет move_zone в наборе")
	}
	payload, _ := json.Marshal(moveIntent)
	dctx := store.DiceCtx{Seed: 1, Turn: 1}
	_, isNew, err := st.AppendCommand(context.Background(), store.CommandLogEntry{
		SessionID:      store.SessionID(id),
		SnapshotID:     rt1.snapshot,
		CoreVersion:    core.Version,
		Intent:         payload,
		DiceCtx:        dctx,
		IdempotencyKey: idempotencyKey(store.SessionID(id), dctx, payload),
	})
	if err != nil || !isNew {
		t.Fatalf("подготовка pending: isNew=%v err=%v", isNew, err)
	}

	// «Рестарт»: восстановление обязано доиграть pending-хвост.
	m2 := NewManagerWithStore(casesRoot, st)
	rt2, ok := m2.Get(id)
	if !ok {
		t.Fatal("сессия не восстановилась")
	}
	if rt2.game.Node != moveIntent.Args.Node {
		t.Fatalf("pending move не доигран: узел %s, ждали %s",
			rt2.game.Node, moveIntent.Args.Node)
	}
	// Команда теперь applied, реплей второй раз её не применит.
	cmds, _ := st.Commands(context.Background(), store.SessionID(id))
	for _, c := range cmds {
		if c.Status != store.CommandApplied {
			t.Fatalf("команда seq %d осталась %s после реплея", c.Seq, c.Status)
		}
	}
}

// Реплей не применяет ход дважды: число применённых команд равно числу ходов,
// а не удвоенному (ключ идемпотентности держит границу).
func TestReplayIsIdempotent(t *testing.T) {
	st := NewMemStore()
	m1 := NewManagerWithStore(casesRoot, st)
	id, _ := m1.Create("harbour", 1)
	rt1, _ := m1.Get(id)
	playTurns(t, rt1, 2)

	// Дважды подряд «рестартим»: каждый реплей идёт по журналу, но команд в
	// журнале остаётся ровно две.
	NewManagerWithStore(casesRoot, st).Get(id)
	NewManagerWithStore(casesRoot, st).Get(id)

	cmds, _ := st.Commands(context.Background(), store.SessionID(id))
	if len(cmds) != 2 {
		t.Fatalf("в журнале %d команд, ждали 2 (реплей дописал команды?)", len(cmds))
	}
}
