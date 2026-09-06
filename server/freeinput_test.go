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
	reply   string
	probe   core.Probe
	clarify string
	idle    bool
	err     error
}

func (f fakeInterp) InterpretChat(_ context.Context, _ string, _ store.EntityID, _ string) (*core.Intent, string, core.Probe, string, bool, error) {
	return f.intent, f.reply, f.probe, f.clarify, f.idle, f.err
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

// Свободный разговорный ход: подводка Мастера (chat-reply) идёт gm-сегментом
// перед прямой речью NPC — вместо выдумывающего обрамления. В ленте [player,
// gm(подводка), npc].
func TestFreeInputTalkLeadInThenReply(t *testing.T) {
	rt, id := freeSession(t, nil)
	rt.mu.Lock()
	talk := rt.game.Affordances(rt.spokenTo)[0].Intent // разговорный ход к NPC
	rt.interp = fakeInterp{intent: &talk, reply: "Вы поворачиваетесь к Берну."}
	rt.mu.Unlock()

	sub := rt.subscribe()
	rt.applyInput(context.Background(), 1, inputPayload{Text: "как дела?"})
	frames := collectUntilDone(t, sub)

	var roles []string
	for _, f := range frames {
		if f.Op == OpStart {
			var ps proseStart
			_ = json.Unmarshal(f.Payload, &ps)
			roles = append(roles, ps.Role)
		}
	}
	// Первый OpStart — лоадер перед разбором; значимые сегменты — подводка gm и
	// реплика npc, и они последние два по порядку.
	if n := len(roles); n < 2 || roles[n-2] != RoleGM || roles[n-1] != RoleNPC {
		t.Fatalf("ждали ...[gm(подводка), npc] в OpStart, получили %v", roles)
	}
	e := transcriptRoles(t, rt, id)
	if n := len(e); n < 3 || e[n-3].Role != RolePlayer || e[n-2].Role != RoleGM || e[n-1].Role != RoleNPC {
		t.Fatalf("лента = %+v, ждали ...[player, gm, npc]", e)
	}
	if e[len(e)-2].Text != "Вы поворачиваетесь к Берну." {
		t.Fatalf("подводка в ленте = %q", e[len(e)-2].Text)
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
