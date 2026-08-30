package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kliuchnikovv/dnd/view"
)

// waitOpeningDone ждёт, пока детач-горутина опенинга допишет буфер и снимет
// признак генерации.
func waitOpeningDone(t *testing.T, rt *sessionRuntime) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		rt.mu.Lock()
		done := !rt.narrating && len(rt.narration) > 0
		rt.mu.Unlock()
		if done {
			return
		}
		select {
		case <-deadline:
			t.Fatal("опенинг-проза не завершилась")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Create запускает вводную прозу места: буфер заполняется без единого хода.
func TestOpeningProseOnCreate(t *testing.T) {
	m := narratorManager(t, "Причал тонет в тумане, доски скрипят")
	id, err := m.Create("harbour", 1, "u")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rt, _ := m.Get(id)
	waitOpeningDone(t, rt)

	rt.mu.Lock()
	got := strings.Join(rt.narration, "")
	rt.mu.Unlock()
	if !strings.Contains(got, "Причал") {
		t.Fatalf("опенинг без прозы: %q", got)
	}
}

// Первый attach отдаёт готовую вводную прозу кадром history.
func TestOpeningProseDeliveredOnAttach(t *testing.T) {
	m := narratorManager(t, "Причал тонет в тумане")
	id, _ := m.Create("harbour", 1, "u")
	rt, _ := m.Get(id)
	waitOpeningDone(t, rt)

	sub := rt.attach()
	var text string
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case f := <-sub.out:
			if f.Op == OpTranscript {
				var tp transcriptPayload
				if err := json.Unmarshal(f.Payload, &tp); err != nil {
					t.Fatalf("transcript payload: %v", err)
				}
				for _, e := range tp.Entries {
					if e.Role == RoleGM {
						text = e.Text
					}
				}
				break loop
			}
		case <-deadline:
			t.Fatal("нет transcript-кадра с опенингом")
		}
	}
	if !strings.Contains(text, "Причал") {
		t.Fatalf("опенинг не доставлен: %q", text)
	}
}

// Живой ход шлёт OpStart (сигнал лоадера) раньше любой дельты прозы.
func TestProseStartPrecedesDeltas(t *testing.T) {
	m := narratorManager(t, "Причал тонет в тумане")
	id, _ := m.Create("harbour", 1, "u")
	rt, _ := m.Get(id)
	sub := rt.subscribe()

	rt.mu.Lock()
	tok := view.OptionToken(rt.game.Affordances(rt.spokenTo)[0])
	rt.mu.Unlock()
	rt.applyInput(context.Background(), 1, inputPayload{Token: tok})

	frames := collectUntilDone(t, sub)
	startAt, firstDeltaAt := -1, -1
	for i, f := range frames {
		if f.Op == OpStart && startAt == -1 {
			startAt = i
		}
		if f.Op == OpMessage && f.Kind == KindData && firstDeltaAt == -1 {
			firstDeltaAt = i
		}
	}
	if startAt == -1 {
		t.Fatal("нет кадра OpStart")
	}
	if firstDeltaAt == -1 || startAt >= firstDeltaAt {
		t.Fatalf("OpStart(%d) должен идти раньше первой дельты(%d)", startAt, firstDeltaAt)
	}
}

// Без narrator Create не роняется и буфер прозы пуст (механика-only).
func TestOpeningProseSkippedWithoutNarrator(t *testing.T) {
	m := NewManager(casesRoot) // без WithNarrator
	id, err := m.Create("harbour", 1, "u")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rt, _ := m.Get(id)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.narration) != 0 || rt.narrating {
		t.Fatalf("без narrator проза стартовать не должна")
	}
}
