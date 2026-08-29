package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/view"
)

// narratorManager — менеджер со стримом прозы на fake-шлюзе. Проза приходит
// словами, детерминированно.
func narratorManager(t *testing.T, reply string) *Manager {
	t.Helper()
	f := llm.NewFake("fake", true).ReplyWith(func(llm.Request) string { return reply })
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleNarrator, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	return NewManager(casesRoot).WithNarrator(master.New(gw))
}

// collectUntilDone читает кадры подписчика до op:done (или таймаута).
func collectUntilDone(t *testing.T, sub *subscriber) []Frame {
	t.Helper()
	var got []Frame
	deadline := time.After(3 * time.Second)
	for {
		select {
		case f := <-sub.out:
			got = append(got, f)
			if f.Op == OpDone {
				return got
			}
		case <-deadline:
			t.Fatalf("не дождались op:done; собрано %d кадров", len(got))
		}
	}
}

// Ход даёт session_state мгновенно, затем прозу дельтами и op:done.
func TestProseStreamsAfterSessionState(t *testing.T) {
	m := narratorManager(t, "Причал тонет в тумане, доски скрипят под ногой")
	id, _ := m.Create("harbour", 1, "test-user")
	rt, _ := m.Get(id)

	sub := rt.subscribe()
	// Первый вариант хода.
	rt.mu.Lock()
	tok := view.OptionToken(rt.game.Affordances(rt.spokenTo)[0])
	rt.mu.Unlock()

	ok, msg := rt.applyInput(context.Background(), 1, inputPayload{Token: tok})
	if !ok {
		t.Fatalf("ход не применился: %s", msg)
	}

	frames := collectUntilDone(t, sub)

	// Первым — session_state (механика мгновенно).
	if frames[0].Op != OpSessionState {
		t.Fatalf("первый кадр %q, ждали session_state", frames[0].Op)
	}
	// Затем дельты прозы, затем done.
	deltas := 0
	var text string
	for _, f := range frames {
		if f.Op == OpMessage && f.Kind == KindData {
			var d textDelta
			if err := json.Unmarshal(f.Payload, &d); err != nil {
				t.Fatalf("дельта не разобралась: %v", err)
			}
			if d.Type != "text" {
				t.Fatalf("тип дельты %q", d.Type)
			}
			text += d.Delta
			deltas++
		}
	}
	if deltas < 2 {
		t.Fatalf("ждали несколько дельт прозы, пришло %d", deltas)
	}
	if text == "" {
		t.Fatalf("проза пустая")
	}
	if frames[len(frames)-1].Op != OpDone {
		t.Fatalf("последним ждали done")
	}
}

// Обрыв одного подписчика не роняет генерацию: второй подписчик всё равно
// получает прозу и done.
func TestProseSurvivesSubscriberDrop(t *testing.T) {
	m := narratorManager(t, "Замок поддаётся, петли стонут, дверь идёт внутрь")
	id, _ := m.Create("harbour", 1, "test-user")
	rt, _ := m.Get(id)

	// Живой подписчик и «мёртвый» (переполненный буфер имитирует отставшего).
	live := rt.subscribe()
	dead := rt.subscribe()
	for i := 0; i < cap(dead.out); i++ {
		dead.out <- Frame{}
	}

	rt.mu.Lock()
	tok := view.OptionToken(rt.game.Affordances(rt.spokenTo)[0])
	rt.mu.Unlock()
	rt.applyInput(context.Background(), 1, inputPayload{Token: tok})

	// Живой доходит до done, несмотря на захлебнувшегося соседа.
	frames := collectUntilDone(t, live)
	if frames[len(frames)-1].Op != OpDone {
		t.Fatalf("живой подписчик не получил done")
	}
}

// По завершении генерации буфер прозы хода заполнен, а признак «идёт» снят —
// основа досстрима при возобновлении (фаза 5).
func TestProseBufferFilledAfterDone(t *testing.T) {
	m := narratorManager(t, "Причал тонет в тумане, доски скрипят под ногой")
	id, _ := m.Create("harbour", 1, "test-user")
	rt, _ := m.Get(id)
	sub := rt.subscribe()

	rt.mu.Lock()
	tok := view.OptionToken(rt.game.Affordances(rt.spokenTo)[0])
	rt.mu.Unlock()
	rt.applyInput(context.Background(), 1, inputPayload{Token: tok})
	collectUntilDone(t, sub)

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.narrating {
		t.Fatalf("признак генерации не снят после done")
	}
	if len(rt.narration) == 0 {
		t.Fatalf("буфер прозы пуст после завершения")
	}
}

// stopProse безопасен и когда генерации нет (двойной op:stop, стоп до хода).
func TestStopProseIsSafeWithoutGeneration(t *testing.T) {
	m := narratorManager(t, "неважно")
	id, _ := m.Create("harbour", 1, "test-user")
	rt, _ := m.Get(id)
	rt.stopProse() // не должно паниковать
}

// Сквозь настоящий сокет: ход даёт session_state, затем прозу дельтами и done.
func TestWSProseEndToEnd(t *testing.T) {
	m := narratorManager(t, "Причал тонет в тумане, доски скрипят под ногой")
	srv := New(m)
	id, _ := m.Create("harbour", 1, "test-user")
	conn, done := wsDial(t, srv, id, "dev")
	defer done()

	start := decodeView(t, readFrame(t, conn))
	writeInput(t, conn, 1, inputPayload{Token: start.Options[0].Token})

	var text string
	deltas, sawDone := 0, false
	for i := 0; i < 200 && !sawDone; i++ {
		f := readFrame(t, conn)
		switch {
		case f.Op == OpMessage && f.Kind == KindData:
			var d textDelta
			if err := json.Unmarshal(f.Payload, &d); err != nil {
				t.Fatalf("дельта не разобралась: %v", err)
			}
			text += d.Delta
			deltas++
		case f.Op == OpDone:
			sawDone = true
		}
	}
	if deltas < 2 {
		t.Fatalf("ждали дельты прозы через сокет, пришло %d", deltas)
	}
	if !sawDone {
		t.Fatalf("не пришёл op:done")
	}
	if !strings.Contains(text, "туман") {
		t.Fatalf("проза через сокет потерялась: %q", text)
	}
}
