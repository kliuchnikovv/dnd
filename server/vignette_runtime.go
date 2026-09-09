package server

import (
	"context"
	"strings"
	"sync"

	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/vignette"
)

// vignetteRuntime — живая сессия ВИНЬЕТКИ. Параллельный движок рядом с
// sessionRuntime (M1a): у виньетки своё состояние/ход (см. пакет vignette),
// которые не ложатся в core.Game, поэтому это отдельный драйвер, а не тот же
// рантайм. Переиспользует лишь движко-нейтральную обвязку: Frame/subscriber/
// broadcast и master.Master-нарратор.
//
// Анти-лик (ADR-0008) конструкцией: Мастеру уходят ТОЛЬКО res.Revealed —
// правды сцены (Scene.Truth) и закрытых тиров он не видит. narrator==nil —
// офлайн-путь без ключа: revealed печатается напрямую.
//
// Персистентность НАМЕРЕННО минимальна (ADR-0009-срез): сессия живёт в памяти и
// проходит сцену целиком; в ленту пишется ход. TODO(second-pass): WAL команд и
// реплей виньетки через Adjudicate для возобновления после рестарта.
type vignetteRuntime struct {
	chatID   string
	userID   string
	seed     int64
	store    Store
	narrator *master.Master

	mu     sync.Mutex
	scene  *vignette.Scene
	state  *vignette.State
	judge  vignette.Judge
	guard  vignette.Guard
	turn   int
	ended  bool
	subs   map[*subscriber]struct{}
	outSeq int

	lastAppliedID int

	// introText — предварительно нарисованная вступительная проза Мастера
	// (холодный вход: только поверхности и Intro, без перцепции). Пусто, если
	// не рендерилась (нет narrator и нет scene.Intro) или ещё не пре-считана.
	// Идея: сгенерировать один раз при Create и отдавать первому подписчику ДО
	// snapshotFrameLocked, чтобы игрок открывал сцену уже с интро (§2.1
	// хендоффа 2026-09-09-vignette-into-mvp).
	introText string
	// introSent — уже отдали интро в out хотя бы одному подписчику. Второй
	// attach (реконнект) интро не получает — как в приключении: опенинг не
	// перепечатывается на реконнекте.
	introSent bool
}

// vignetteView — тело OpSessionState для виньетки: минимальный снимок сцены и
// хода. Ни правды, ни закрытых тиров.
type vignetteView struct {
	Scene     string   `json:"scene"`
	Surfaces  []string `json:"surfaces"`
	Revealed  []string `json:"revealed"`
	StateNote string   `json:"state_note"`
	Beat      string   `json:"beat,omitempty"`
	Ended     bool     `json:"ended"`
	EndText   string   `json:"end_text,omitempty"`
}

func newVignetteRuntime(chatID, userID string, seed int64, st Store, narrator *master.Master,
	sc *vignette.Scene, state *vignette.State) *vignetteRuntime {
	return &vignetteRuntime{
		chatID: chatID, userID: userID, seed: seed, store: st, narrator: narrator,
		scene: sc, state: state, judge: vignette.KeywordJudge{}, guard: vignette.KeywordGuard{},
		subs: map[*subscriber]struct{}{},
	}
}

// ── обвязка подписки/рассылки (своя копия — движко-нейтральная, ~M1a) ──

func (rt *vignetteRuntime) subscribe() *subscriber {
	sub := &subscriber{out: make(chan Frame, 64)}
	rt.mu.Lock()
	rt.subs[sub] = struct{}{}
	rt.mu.Unlock()
	return sub
}

func (rt *vignetteRuntime) unsubscribe(sub *subscriber) {
	rt.mu.Lock()
	delete(rt.subs, sub)
	rt.mu.Unlock()
}

func (rt *vignetteRuntime) broadcastLocked(f Frame) {
	for sub := range rt.subs {
		select {
		case sub.out <- f:
		default:
		}
	}
}

func (rt *vignetteRuntime) nextOutIDLocked() int {
	rt.outSeq++
	return rt.outSeq
}

// attach — (ре)коннект: подписаться и сразу получить текущий снимок. Первый
// attach также получает вступительную прозу Мастера (см. introText): интро идёт
// ДО snapshotFrame, чтобы клиент рисовал прозу первой, а сцену — уже под ней.
func (rt *vignetteRuntime) attach() *subscriber {
	sub := rt.subscribe()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.introSent && strings.TrimSpace(rt.introText) != "" {
		rt.emitIntroLocked(sub)
		rt.introSent = true
	}
	sub.out <- rt.snapshotFrameLocked()
	return sub
}

// emitIntroLocked отдаёт вступительную прозу конкретному подписчику как
// одно-сегментный стрим: OpStart(role=gm) → OpMessage(delta) → OpDone. Пишет в
// ленту сессии (RoleGM), чтобы в журнале интро было ровно один раз (первый
// attach; последующие реконнекты берут ленту из истории). Под rt.mu.
func (rt *vignetteRuntime) emitIntroLocked(sub *subscriber) {
	sub.out <- newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindMeta, OpStart,
		proseStart{Role: RoleGM})
	sub.out <- newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindData, OpMessage,
		textDelta{Type: "text", Delta: rt.introText, IX: 0})
	sub.out <- newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindMeta, OpDone, nil)
	rt.appendTranscript(TranscriptEntry{Role: RoleGM, Text: rt.introText})
}

// precomputeIntro рисует вступительную прозу через narrator и кладёт её в
// introText под rt.mu. Идемпотентна: повторный вызов ничего не делает. Пустой
// результат означает «нечего показать» (нет narrator и scene.Intro пуст).
func (rt *vignetteRuntime) precomputeIntro(ctx context.Context) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.introText != "" || rt.introSent {
		return
	}
	rt.introText = rt.renderIntroLocked(ctx)
}

// renderIntroLocked собирает холодный вход в сцену: место + Intro в поверхности
// (прото-паттерн introInput), revealed пусто (перцепции ещё не было). narrator
// nil (офлайн без ключа) — фолбэк на scene.Intro как есть. Страж применяется
// всегда — та же защита ADR-0008, что у пер-ходовой прозы. Под rt.mu.
func (rt *vignetteRuntime) renderIntroLocked(ctx context.Context) string {
	if rt.scene == nil {
		return ""
	}
	var text string
	if rt.narrator != nil {
		n := masterNarrator{rt.narrator}
		surfaces := rt.scene.Surfaces()
		introSurfaces := make([]string, 0, len(surfaces)+1)
		if s := strings.TrimSpace(rt.scene.Intro); s != "" {
			introSurfaces = append(introSurfaces, s)
		}
		introSurfaces = append(introSurfaces, surfaces...)
		frame := "Опиши холодный вход в сцену: место, положение, атмосферу. Пиши коротко."
		if out, err := n.Narrate(ctx, rt.scene.Ambient, introSurfaces, nil, frame); err == nil {
			text = out
		}
	}
	if strings.TrimSpace(text) == "" {
		text = rt.scene.Intro
	}
	if rt.guard != nil && strings.TrimSpace(text) != "" {
		text = rt.guard.Check(text,
			vignette.ProtectedFacts(rt.scene, rt.state),
			vignette.StateFacts(rt.scene, rt.state),
			nil).Clean
	}
	return strings.TrimSpace(text)
}

func (rt *vignetteRuntime) snapshotFrameLocked() Frame {
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindData, OpSessionState,
		vignetteView{
			Scene:    rt.scene.Title,
			Surfaces: rt.scene.Surfaces(),
			Ended:    rt.ended,
		})
}

// ── ход ──

// applyInput — один ход виньетки: текст игрока → судья → Ruling → Adjudicate →
// снимок состояния + проза (только revealed). Возвращает (ok, ошибка) как
// sessionRuntime.
func (rt *vignetteRuntime) applyInput(ctx context.Context, frameID int, in inputPayload) (bool, string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if frameID != 0 && frameID <= rt.lastAppliedID {
		rt.broadcastLocked(rt.snapshotFrameLocked())
		return true, ""
	}
	if rt.ended {
		rt.broadcastLocked(rt.snapshotFrameLocked())
		return true, ""
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return false, "пустой ввод"
	}
	if frameID != 0 {
		rt.lastAppliedID = frameID
	}

	// hold считает достоянные ходы: Turn растёт на каждом применённом ходу.
	rt.state.Turn++
	ruling := rt.judge.Rule(text, vignette.BuildJudgeView(rt.scene, rt.state))
	res := rt.state.Adjudicate(rt.scene, ruling)
	rt.turn++

	// Эхо действия игрока в ленту.
	rt.appendTranscript(TranscriptEntry{Role: RolePlayer, Text: text})

	// Снимок состояния — механика у клиента сразу. НО на завершающем ходу
	// ended/end_text ПРОПУСКАЕМ здесь: клиент не должен видеть занавес до того,
	// как приедет финальная проза Мастера (см. §2.2 хендоффа
	// 2026-09-09-vignette-into-mvp — гонка порядка кадров). Ended уходит после
	// narrateLocked отдельным финальным session_state.
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindData, OpSessionState,
		vignetteView{
			Scene: rt.scene.Title, Surfaces: rt.scene.Surfaces(),
			Revealed: res.Revealed, StateNote: res.StateNote, Beat: res.Beat,
		}))

	if res.Ended {
		rt.ended = true
	}
	// narrateLocked сам вставит финальный ended-кадр между OpMessage и OpDone,
	// если res.Ended — так гарантируется порядок «проза → занавес» на одном
	// потоке (OpDone идёт последним, и тесты, читающие «до op:done», ловят
	// финальный session_state).
	rt.narrateLocked(ctx, res)
	return true, ""
}

// masterNarrator адаптирует *master.Master к vignette.Narrator: строит World из
// нейтральных поверхностей и зовёт Narrate с ТОЛЬКО revealed (анти-лик ADR-0008).
type masterNarrator struct{ m *master.Master }

func (mn masterNarrator) Narrate(ctx context.Context, ambient string, surfaces, revealed []string, frame string) (string, error) {
	return mn.m.Narrate(ctx, master.KindOutcome, frame,
		master.World{Setting: ambient, Scene: surfaces}, revealed, "", nil, llm.Request{})
}

// narrateLocked печатает прозу хода. Пер-ходовая проза+страж — ТОТ ЖЕ код, что у
// CLI: vignette.Narrate (Мастеру уходит ТОЛЬКО revealed; страж с карваутом финала).
// narrator==nil — офлайн без ключа: revealed печатается напрямую (adapter не строим).
func (rt *vignetteRuntime) narrateLocked(ctx context.Context, res vignette.Result) {
	var n vignette.Narrator
	if rt.narrator != nil {
		n = masterNarrator{rt.narrator}
	}
	text, err := vignette.Narrate(ctx, rt.scene, rt.state, res, n, rt.guard)
	if err != nil {
		rt.broadcastLocked(errorFrame(rt.nextOutIDLocked(), rt.chatID, "проза хода не удалась: "+err.Error()))
		return
	}
	if strings.TrimSpace(text) == "" {
		return
	}

	// Простой одно-сегментный стрим: start → одна дельта → [ended?] → done.
	// Финальный session_state{ended,end_text} вклинивается МЕЖДУ дельтой и
	// done: OpDone остаётся последним кадром хода (тесты читают до op:done),
	// а занавес всегда после прозы (§2.2 хендоффа 2026-09-09-vignette-into-mvp).
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindMeta, OpStart,
		proseStart{Role: RoleGM}))
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindData, OpMessage,
		textDelta{Type: "text", Delta: text, IX: 0}))
	if res.Ended {
		rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindData, OpSessionState,
			vignetteView{
				Scene: rt.scene.Title, Surfaces: rt.scene.Surfaces(),
				Revealed: res.Revealed, StateNote: res.StateNote, Beat: res.Beat,
				Ended: true, EndText: res.EndText,
			}))
	}
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindMeta, OpDone, nil))
	rt.appendTranscript(TranscriptEntry{Role: RoleGM, Text: text})
}

func (rt *vignetteRuntime) appendTranscript(e TranscriptEntry) {
	if rt.store == nil {
		return
	}
	_ = rt.store.AppendTranscript(context.Background(), store.SessionID(rt.chatID), e)
}

// stopProse — отмена прозы. Проза виньетки синхронна и однократна, отменять
// нечего; метод есть для симметрии с sessionRuntime (WS-слой).
func (rt *vignetteRuntime) stopProse() {}
