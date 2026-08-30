package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// fakeInterp — детерминированный разбор свободного ввода для тестов ветвления
// interpretFreeLocked (реальный разбор — LLM, тестируется в пакете intent).
type fakeInterp struct {
	intent  *core.Intent
	probe   core.Probe
	clarify string
	err     error
}

func (f fakeInterp) InterpretChat(_ context.Context, _ string, _ store.EntityID, _ string) (*core.Intent, string, core.Probe, string, error) {
	return f.intent, "", f.probe, f.clarify, f.err
}

// freeSession поднимает сессию с прозой и подставным разбором свободного ввода.
func freeSession(t *testing.T, fi chatInterpreter) (*sessionRuntime, string) {
	t.Helper()
	m := narratorManager(t, "проза")
	id, _ := m.Create("harbour", 1, "u")
	rt, _ := m.Get(id)
	waitOpeningDone(t, rt) // отделяем опенинг
	rt.mu.Lock()
	rt.interp = fi
	rt.mu.Unlock()
	return rt, id
}

func transcriptRoles(t *testing.T, rt *sessionRuntime, id string) []TranscriptEntry {
	t.Helper()
	e, _ := rt.store.Transcript(context.Background(), store.SessionID(id))
	return e
}

// Свободный текст, разобранный в интент → обычный ход: session_state + проза,
// эхо действия в ленте — сам текст игрока.
func TestFreeInputIntentApplies(t *testing.T) {
	rt, _ := freeSession(t, nil)
	// Берём валидный интент из показанного набора.
	rt.mu.Lock()
	valid := rt.game.Affordances(rt.spokenTo)[0].Intent
	rt.interp = fakeInterp{intent: &valid}
	rt.mu.Unlock()

	sub := rt.subscribe()
	ok, msg := rt.applyInput(context.Background(), 1, inputPayload{Text: "осмотреться внимательно"})
	if !ok {
		t.Fatalf("свободный ход не применился: %s", msg)
	}
	frames := collectUntilDone(t, sub)
	var sawState bool
	for _, f := range frames {
		if f.Op == OpSessionState {
			sawState = true
		}
	}
	if !sawState {
		t.Fatal("интент-ход не прислал session_state")
	}
	e := transcriptRoles(t, rt, rt.chatID)
	var echoed bool
	for _, x := range e {
		if x.Role == RolePlayer && x.Text == "осмотреться внимательно" {
			echoed = true
		}
	}
	if !echoed {
		t.Fatalf("эхо свободного действия не в ленте: %+v", e)
	}
}

// Свободный текст, не давший интента и не задевший цель → проба: мир отвечает
// прозой, ход не тратится (журнала нет), в ленте — эхо + отклик.
func TestFreeInputProbeNarrates(t *testing.T) {
	rt, id := freeSession(t, fakeInterp{probe: core.Probe{Text: "поковырять зыбкую пустоту"}})

	sub := rt.subscribe()
	ok, _ := rt.applyInput(context.Background(), 1, inputPayload{Text: "поковырять зыбкую пустоту"})
	if !ok {
		t.Fatal("проба должна пройти как валидный ввод")
	}
	collectUntilDone(t, sub)

	e := transcriptRoles(t, rt, id)
	if n := len(e); n < 2 || e[n-2].Role != RolePlayer || e[n-1].Role != RoleGM {
		t.Fatalf("проба не дала [player, gm] в ленте: %+v", e)
	}
	// Хода проба не тратит: команд в журнале нет.
	cmds, _ := rt.store.Commands(context.Background(), store.SessionID(id))
	if len(cmds) != 0 {
		t.Fatalf("проба записала команду в журнал (%d) — ход не должен тратиться", len(cmds))
	}
}

// Неоднозначный текст → уточнение: игроку идёт диегетическая проза (не служебный
// вопрос), подсказка интерпретатора держится во внутреннем pending, ход не
// состоялся (журнал пуст), в ленте — эхо игрока + прозаический отклик.
func TestFreeInputClarifyAsks(t *testing.T) {
	rt, id := freeSession(t, fakeInterp{clarify: "К кому вы обращаетесь?"})

	sub := rt.subscribe()
	rt.applyInput(context.Background(), 1, inputPayload{Text: "спросить"})
	frames := collectUntilDone(t, sub)

	var prose string
	for _, f := range frames {
		if f.Op == OpMessage && f.Kind == KindData {
			var d textDelta
			_ = json.Unmarshal(f.Payload, &d)
			prose += d.Delta
		}
	}
	if prose == "" {
		t.Fatal("уточнение не дало прозы игроку")
	}
	rt.mu.Lock()
	pending := rt.pending
	rt.mu.Unlock()
	if pending != "К кому вы обращаетесь?" {
		t.Fatalf("подсказка интерпретатора не легла во внутренний pending: %q", pending)
	}
	e := transcriptRoles(t, rt, id)
	if n := len(e); n < 2 || e[n-2].Role != RolePlayer || e[n-1].Role != RoleGM {
		t.Fatalf("уточнение не дало [player, gm] в ленте: %+v", e)
	}
	cmds, _ := rt.store.Commands(context.Background(), store.SessionID(id))
	if len(cmds) != 0 {
		t.Fatalf("уточнение записало команду — хода не было")
	}
}

// Сбой переводчика → error-кадр, лоадер снят (не молчим и не виснем).
func TestFreeInputErrorSurfaces(t *testing.T) {
	rt, _ := freeSession(t, fakeInterp{err: context.DeadlineExceeded})

	sub := rt.subscribe()
	rt.applyInput(context.Background(), 1, inputPayload{Text: "что-нибудь"})
	frames := drainReady(sub)

	var sawErr bool
	for _, f := range frames {
		if f.Kind == KindError {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("сбой переводчика не дал error-кадра")
	}
	rt.mu.Lock()
	narrating := rt.narrating
	rt.mu.Unlock()
	if narrating {
		t.Fatal("после ошибки лоадер завис (narrating не снят)")
	}
}
