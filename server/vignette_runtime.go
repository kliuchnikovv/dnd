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

// attach — (ре)коннект: подписаться и сразу получить текущий снимок.
func (rt *vignetteRuntime) attach() *subscriber {
	sub := rt.subscribe()
	rt.mu.Lock()
	sub.out <- rt.snapshotFrameLocked()
	rt.mu.Unlock()
	return sub
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

	// Снимок состояния — механика у клиента сразу.
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindData, OpSessionState,
		vignetteView{
			Scene: rt.scene.Title, Surfaces: rt.scene.Surfaces(),
			Revealed: res.Revealed, StateNote: res.StateNote, Beat: res.Beat,
			Ended: res.Ended, EndText: res.EndText,
		}))

	if res.Ended {
		rt.ended = true
	}
	rt.narrateLocked(ctx, res)
	return true, ""
}

// narrateLocked печатает прозу хода. КЛЮЧЕВОЕ: Мастеру уходит ТОЛЬКО revealed
// (+ нейтральные положение/беат/развязка) — правды и закрытых тиров он не видит.
// narrator==nil — офлайн без ключа: revealed печатается напрямую.
func (rt *vignetteRuntime) narrateLocked(ctx context.Context, res vignette.Result) {
	// outcome — только то, что ядро ОТКРЫЛО: revealed-строки, нейтральное
	// положение, беат-событие и (на конце) раскрытая развязка. Ни Scene.Truth,
	// ни закрытого тира здесь нет.
	outcome := append([]string{}, res.Revealed...)
	if res.StateNote != "" {
		outcome = append(outcome, res.StateNote)
	}
	if res.Beat != "" {
		outcome = append(outcome, res.Beat)
	}
	if res.Ended && res.EndText != "" {
		outcome = append(outcome, res.EndText)
	}

	var text string
	if rt.narrator != nil {
		frame := firstNonEmptyStr(res.Beat, res.EndText, res.StateNote, "Опиши, чем кончился ход.")
		out, err := rt.narrator.Narrate(ctx, master.KindOutcome, frame,
			master.World{Setting: rt.scene.Ambient, Scene: rt.scene.Surfaces()},
			res.Revealed, "", nil, llm.Request{})
		if err != nil {
			rt.broadcastLocked(errorFrame(rt.nextOutIDLocked(), rt.chatID, "проза хода не удалась: "+err.Error()))
			return
		}
		text = out
	}
	if strings.TrimSpace(text) == "" {
		text = strings.Join(outcome, " ")
	}
	// Страж-редактор (ADR-0008): бэкстоп против дословного эха защищённого факта.
	// Держит правду + закрытые тиры; на ended раскрытая развязка идёт в allowed
	// (карваут финала) и не режется.
	if rt.guard != nil {
		var allowed []string
		if res.Ended && res.EndText != "" {
			allowed = []string{res.EndText}
		}
		text = rt.guard.Check(text, vignette.ProtectedFacts(rt.scene, rt.state),
			vignette.StateFacts(rt.scene, rt.state), allowed).Clean
	}
	if strings.TrimSpace(text) == "" {
		return
	}

	// Простой одно-сегментный стрим: start → одна дельта → done.
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindMeta, OpStart,
		proseStart{Role: RoleGM}))
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindData, OpMessage,
		textDelta{Type: "text", Delta: text, IX: 0}))
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

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
