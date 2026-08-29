package server

import (
	"encoding/json"
	"testing"
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

// Реконнект после завершённой прозы: session_state + history с готовым текстом,
// без дельт и без done (ход уже кончился).
func TestAttachFinishedSendsHistory(t *testing.T) {
	m := NewManager(casesRoot)
	id, _ := m.Create("harbour", 1)
	rt, _ := m.Get(id)

	// Имитируем завершённый ход с прозой в буфере.
	rt.mu.Lock()
	rt.narration = []string{"Причал ", "тонет ", "в тумане"}
	rt.narrating = false
	rt.mu.Unlock()

	sub := rt.attach()
	frames := drainReady(sub)

	if frames[0].Op != OpSessionState {
		t.Fatalf("первый кадр %q, ждали session_state", frames[0].Op)
	}
	var history *Frame
	for i := range frames {
		if frames[i].Op == OpHistory {
			history = &frames[i]
		}
		if frames[i].Op == OpMessage && frames[i].Kind == KindData {
			t.Fatalf("завершённый ход прислал дельту вместо history")
		}
		if frames[i].Op == OpDone {
			t.Fatalf("для завершённого хода done не нужен")
		}
	}
	if history == nil {
		t.Fatalf("нет кадра history")
	}
	var hp historyPayload
	if err := json.Unmarshal(history.Payload, &hp); err != nil {
		t.Fatalf("history не разобрался: %v", err)
	}
	if hp.Text != "Причал тонет в тумане" {
		t.Fatalf("history.text = %q", hp.Text)
	}
}

// Реконнект в середине генерации: session_state + догон буферных дельт, без
// history и без done (остаток и done доедут живой рассылкой).
func TestAttachInflightSendsBufferedDeltas(t *testing.T) {
	m := NewManager(casesRoot)
	id, _ := m.Create("harbour", 1)
	rt, _ := m.Get(id)

	rt.mu.Lock()
	rt.narration = []string{"Замок ", "поддаётся"}
	rt.narrating = true // проза в полёте
	rt.mu.Unlock()

	sub := rt.attach()
	frames := drainReady(sub)

	deltas := 0
	for _, f := range frames {
		if f.Op == OpHistory {
			t.Fatalf("в полёте history слать нельзя — придёт дельтами")
		}
		if f.Op == OpDone {
			t.Fatalf("в полёте done рано: остаток ещё идёт")
		}
		if f.Op == OpMessage && f.Kind == KindData {
			deltas++
		}
	}
	if deltas != 2 {
		t.Fatalf("догон дельт %d, ждали 2", deltas)
	}
}

// Реконнект без прозы (свежая сессия): только session_state.
func TestAttachNoProseOnlyState(t *testing.T) {
	m := NewManager(casesRoot)
	id, _ := m.Create("harbour", 1)
	rt, _ := m.Get(id)

	sub := rt.attach()
	frames := drainReady(sub)
	if len(frames) != 1 || frames[0].Op != OpSessionState {
		t.Fatalf("ждали один session_state, получили %d кадров", len(frames))
	}
}

// Сквозь сокет: сыграв ход с прозой на одном соединении, второе (реконнект)
// получает session_state и history с той же прозой.
func TestWSReconnectResumesProse(t *testing.T) {
	m := narratorManager(t, "Причал тонет в тумане, доски скрипят под ногой")
	srv := New(m)
	id, _ := m.Create("harbour", 1)

	// Первое соединение: играем ход, дожидаемся done.
	c1, done1 := wsDial(t, srv, id)
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

	// Реконнект: session_state + history с готовой прозой.
	c2, done2 := wsDial(t, srv, id)
	defer done2()
	if decodeView(t, readFrame(t, c2)).Version == 0 {
		t.Fatalf("реконнект без корректного session_state")
	}
	h := readFrame(t, c2)
	if h.Op != OpHistory {
		t.Fatalf("вторым кадром ждали history, получили %q", h.Op)
	}
	var hp historyPayload
	json.Unmarshal(h.Payload, &hp)
	if hp.Text != first {
		t.Fatalf("history разошёлся с прозой хода:\nход:    %q\nhistory: %q", first, hp.Text)
	}
}
