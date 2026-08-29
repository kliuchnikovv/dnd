package server

import (
	"context"
	"io"

	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
)

// startProseLocked запускает стрим прозы исхода хода. Вызывается под rt.mu из
// applyInput, СРАЗУ ПОСЛЕ того как ушёл session_state: механика у клиента уже
// есть, проза доезжает. Генерация детачится от сокета — идёт горутиной, пишет
// в буфер хода и рассылает подписчикам, поэтому обрыв сокета её не роняет, а
// реконнект (фаза 5) досстримит из буфера.
//
// Аргументы Мастеру строятся ЗДЕСЬ, под rt.mu, из свежего состояния игры —
// горутина игры уже не касается, только шлёт дельты. Нет прозы (отказ, нет
// авторской рамки) — нет и генерации.
func (rt *sessionRuntime) startProseLocked(in core.Intent, res core.TurnResult) {
	if rt.narrator == nil {
		return
	}
	pr, ok := cli.OutcomeProse(rt.game, in, res)
	if !ok {
		return
	}
	setting := rt.game.Setting

	// Суперсессия: новый ход отменяет незавершённую прозу прошлого.
	if rt.genCancel != nil {
		rt.genCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt.genCancel = cancel
	rt.narration = nil
	rt.narrating = true
	rt.narrateGen++
	gen := rt.narrateGen

	go rt.streamProse(ctx, gen, pr, setting)
}

// streamProse гонит Мастера и релеит дельты. gen отсекает пережившую суперсессию
// горутину: дельта из устаревшего поколения в буфер и рассылку не попадает.
func (rt *sessionRuntime) streamProse(ctx context.Context, gen int, pr cli.Prose, setting string) {
	st, err := rt.narrator.NarrateStream(ctx, kindOf(pr.Kind), pr.Frame,
		master.World{Setting: setting, Scene: pr.Scene},
		pr.Outcome, pr.Speaking, pr.State, llm.Request{})
	if err != nil {
		rt.failProse(gen, err)
		return
	}
	defer st.Close()

	ix := 0
	for {
		d, err := st.Recv()
		if d.Text != "" {
			if !rt.emitDelta(gen, d.Text, ix) {
				return // суперсессия или отмена: замолкаем
			}
			ix++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			rt.failProse(gen, err)
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	rt.finishProse(gen)
}

// emitDelta кладёт дельту в буфер и рассылает её. false означает, что это
// поколение устарело (пришёл новый ход) — горутине пора замолчать.
func (rt *sessionRuntime) emitDelta(gen int, text string, ix int) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if gen != rt.narrateGen {
		return false
	}
	rt.narration = append(rt.narration, text)
	id := rt.nextOutIDLocked()
	rt.broadcastLocked(newFrame(id, rt.chatID, ChannelChat, KindData, OpMessage,
		textDelta{Type: "text", Delta: text, IX: ix}))
	return true
}

// finishProse завершает ход кадром done.
func (rt *sessionRuntime) finishProse(gen int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if gen != rt.narrateGen {
		return
	}
	rt.narrating = false
	id := rt.nextOutIDLocked()
	rt.broadcastLocked(newFrame(id, rt.chatID, ChannelChat, KindMeta, OpDone, nil))
}

// failProse сообщает о сбое генерации error-кадром. Механика уже применена
// ядром отдельно от прозы, поэтому канон сбой не портит: клиент остаётся с
// корректным session_state, просто без прозы этого хода.
func (rt *sessionRuntime) failProse(gen int, err error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if gen != rt.narrateGen {
		return
	}
	rt.narrating = false
	rt.broadcastLocked(errorFrame(rt.nextOutIDLocked(), rt.chatID,
		"проза хода не удалась: "+err.Error()))
}

// stopProse отменяет генерацию текущего хода (op:stop). Механику не трогает.
func (rt *sessionRuntime) stopProse() {
	rt.mu.Lock()
	cancel := rt.genCancel
	rt.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// kindOf переводит вид прозы презентации в вид Мастера. Один ход — один вид.
func kindOf(k cli.ProseKind) master.Kind {
	switch k {
	case cli.ProsePlace:
		return master.KindPlace
	case cli.ProseBriefing:
		return master.KindBriefing
	case cli.ProseProbe:
		return master.KindProbe
	default:
		return master.KindOutcome
	}
}
