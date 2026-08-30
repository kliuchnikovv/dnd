package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// drainReady вычитывает всё, что уже лежит в очереди подписчика (без ожидания).
func drainReady(sub *subscriber) []Frame {
	var out []Frame
	for {
		select {
		case f := <-sub.out:
			out = append(out, f)
		default:
			return out
		}
	}
}

// decodeTranscript находит кадр transcript и разбирает его.
func decodeTranscript(t *testing.T, frames []Frame) transcriptPayload {
	t.Helper()
	for _, f := range frames {
		if f.Op == OpTranscript {
			var tp transcriptPayload
			if err := json.Unmarshal(f.Payload, &tp); err != nil {
				t.Fatalf("transcript не разобрался: %v", err)
			}
			return tp
		}
	}
	t.Fatalf("нет кадра transcript среди %d кадров", len(frames))
	return transcriptPayload{}
}

// Реконнект отдаёт всю сохранённую историю кадром transcript (session_state +
// transcript, без дельт и без done для завершённого хода).
func TestAttachSendsTranscript(t *testing.T) {
	m := NewManager(casesRoot) // без narrator: опенинг не пишется, лента чистая
	id, _ := m.Create("harbour", 1, "test-user")
	rt, _ := m.Get(id)

	sid := store.SessionID(id)
	ctx := context.Background()
	_ = rt.store.AppendTranscript(ctx, sid, TranscriptEntry{Role: RolePlayer, Text: "осмотреть журнал"})
	_ = rt.store.AppendTranscript(ctx, sid, TranscriptEntry{Role: RoleGM, Text: "Причал тонет в тумане"})

	sub := rt.attach()
	frames := drainReady(sub)

	if frames[0].Op != OpSessionState {
		t.Fatalf("первый кадр %q, ждали session_state", frames[0].Op)
	}
	for _, f := range frames {
		if f.Op == OpMessage && f.Kind == KindData {
			t.Fatalf("для завершённой истории дельт быть не должно")
		}
		if f.Op == OpDone {
			t.Fatalf("done для завершённой истории не нужен")
		}
	}
	tp := decodeTranscript(t, frames)
	if len(tp.Entries) != 2 {
		t.Fatalf("записей ленты %d, ждали 2", len(tp.Entries))
	}
	if tp.Entries[0].Role != RolePlayer || tp.Entries[0].Text != "осмотреть журнал" {
		t.Fatalf("первая запись = %+v", tp.Entries[0])
	}
	if tp.Entries[1].Role != RoleGM || tp.Entries[1].Text != "Причал тонет в тумане" {
		t.Fatalf("вторая запись = %+v", tp.Entries[1])
	}
}

// Реконнект в середине генерации: session_state + OpStart + догон буферных дельт,
// без done (остаток и done доедут живой рассылкой).
func TestAttachInflightSendsBufferedDeltas(t *testing.T) {
	m := NewManager(casesRoot)
	id, _ := m.Create("harbour", 1, "test-user")
	rt, _ := m.Get(id)

	rt.mu.Lock()
	rt.narration = []string{"Замок ", "поддаётся"}
	rt.narrating = true // проза в полёте
	rt.mu.Unlock()

	sub := rt.attach()
	frames := drainReady(sub)

	start, deltas := 0, 0
	for _, f := range frames {
		if f.Op == OpStart {
			start++
		}
		if f.Op == OpDone {
			t.Fatalf("в полёте done рано: остаток ещё идёт")
		}
		if f.Op == OpMessage && f.Kind == KindData {
			deltas++
		}
	}
	if start != 1 {
		t.Fatalf("ждали один OpStart (лоадер), получили %d", start)
	}
	if deltas != 2 {
		t.Fatalf("догон дельт %d, ждали 2", deltas)
	}
}

// Реконнект без прозы и без истории (свежая сессия): только session_state.
func TestAttachNoProseOnlyState(t *testing.T) {
	m := NewManager(casesRoot)
	id, _ := m.Create("harbour", 1, "test-user")
	rt, _ := m.Get(id)

	sub := rt.attach()
	frames := drainReady(sub)
	if len(frames) != 1 || frames[0].Op != OpSessionState {
		t.Fatalf("ждали один session_state, получили %d кадров", len(frames))
	}
}

// Сквозь сокет: сыграв ход с прозой на одном соединении, второе (реконнект)
// получает session_state и transcript с эхом действия и той же прозой.
func TestWSReconnectResumesProse(t *testing.T) {
	m := narratorManager(t, "Причал тонет в тумане, доски скрипят под ногой")
	srv := New(m)
	id, _ := m.Create("harbour", 1, "test-user")

	// Первое соединение: играем ход, дожидаемся done.
	c1, done1 := wsDial(t, srv, id, "dev")
	start := decodeView(t, readFrame(t, c1))
	writeInput(t, c1, 1, inputPayload{Token: start.Options[0].Token})
	var first string
	for i := 0; i < 200; i++ {
		f := readFrame(t, c1)
		if f.Op == OpMessage && f.Kind == KindData {
			var d textDelta
			json.Unmarshal(f.Payload, &d)
			first += d.Delta
		}
		if f.Op == OpDone {
			break
		}
	}
	done1()

	// Реконнект: session_state + transcript с прозой хода.
	c2, done2 := wsDial(t, srv, id, "dev")
	defer done2()
	if decodeView(t, readFrame(t, c2)).Version == 0 {
		t.Fatalf("реконнект без корректного session_state")
	}
	tf := readFrame(t, c2)
	if tf.Op != OpTranscript {
		t.Fatalf("вторым кадром ждали transcript, получили %q", tf.Op)
	}
	var tp transcriptPayload
	json.Unmarshal(tf.Payload, &tp)
	var gm string
	for _, e := range tp.Entries {
		if e.Role == RoleGM {
			gm = e.Text
		}
	}
	if gm != first {
		t.Fatalf("проза в ленте разошлась с ходом:\nход:  %q\nлента: %q", first, gm)
	}
}
