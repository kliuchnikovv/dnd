package server

import (
	"context"
	"io"
	"log"
	"strings"

	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/store"
)

// proseSegment — одна часть прозы хода: заказ Мастеру + роль/говорящий, под
// которыми она уйдёт в ленту и в OpStart. Разговорный ход — два сегмента:
// обрамление (gm, Мастер описывает обстановку, НЕ озвучивая NPC) и реплика
// (npc, прямая речь персонажа). Обычный ход и опенинг — один сегмент (gm).
type proseSegment struct {
	pr      cli.Prose
	role    string
	speaker string
}

// proseStart — тело кадра OpStart: под какой ролью/именем пойдёт следующий
// сегмент прозы. Клиент по нему открывает нужный блок (Мастер или реплика NPC).
type proseStart struct {
	Role    string `json:"role"`
	Speaker string `json:"speaker,omitempty"`
}

// startProseLocked собирает сегменты прозы хода и запускает их стрим. Вызывается
// под rt.mu из applyInput, СРАЗУ ПОСЛЕ session_state: механика у клиента уже
// есть, проза доезжает. Разговорный ход даёт два сегмента: обрамление исхода и
// прямую речь NPC. Нет прозы (отказ, нет авторской рамки) — нет и генерации.
func (rt *sessionRuntime) startProseLocked(in core.Intent, res core.TurnResult) {
	if rt.narrator == nil {
		return
	}
	var segs []proseSegment
	if fr, ok := cli.OutcomeProse(rt.game, in, res); ok {
		segs = append(segs, proseSegment{pr: fr, role: RoleGM})
	}
	if rp, ok := cli.ReplyProse(rt.game, in, res); ok {
		segs = append(segs, proseSegment{pr: rp, role: RoleNPC, speaker: rp.Speaking})
	}
	rt.launchSegmentsLocked(segs)
}

// startOpeningProseLocked стримит вводную прозу места при рождении сессии, чтобы
// первый экран нёс описание, а не только механику — как открытие партии в CLI
// (Render.Scene). Без narrator — тихо ничего. Под rt.mu.
func (rt *sessionRuntime) startOpeningProseLocked() {
	if rt.narrator == nil {
		return
	}
	rt.launchSegmentsLocked([]proseSegment{{pr: cli.PlaceProse(rt.game), role: RoleGM}})
}

// launchSegmentsLocked отменяет незавершённую прозу прошлого поколения, сбрасывает
// буфер и детачит горутину, стримящую сегменты по очереди. Под rt.mu.
func (rt *sessionRuntime) launchSegmentsLocked(segs []proseSegment) {
	if len(segs) == 0 {
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
	rt.narrateRole = segs[0].role
	rt.narrateSpeaker = segs[0].speaker
	gen := rt.narrateGen

	// Сигнал «генерю» с ролью первого сегмента: клиент поднимает лоадер до первой
	// дельты. Опенинг — подписчиков ещё нет, кадр теряется; его дошлёт attach().
	rt.broadcastLocked(rt.startFrameLocked(segs[0].role, segs[0].speaker))

	go rt.streamSegments(ctx, gen, segs, setting)
}

// startFrameLocked собирает кадр OpStart с ролью/говорящим сегмента. Под rt.mu.
func (rt *sessionRuntime) startFrameLocked(role, speaker string) Frame {
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat, KindMeta, OpStart,
		proseStart{Role: role, Speaker: speaker})
}

// streamSegments гонит сегменты по очереди: каждый завершённый уходит в ленту, на
// границе шлётся новый OpStart, в конце — общий done. gen отсекает пережившую
// суперсессию горутину.
func (rt *sessionRuntime) streamSegments(ctx context.Context, gen int, segs []proseSegment, setting string) {
	for i, seg := range segs {
		if i > 0 && !rt.beginSegment(gen, seg.role, seg.speaker) {
			return // суперсессия/отмена между сегментами
		}
		st, err := rt.narrator.NarrateStream(ctx, kindOf(seg.pr.Kind), seg.pr.Frame,
			master.World{Setting: setting, Scene: seg.pr.Scene},
			seg.pr.Outcome, seg.pr.Speaking, seg.pr.State, llm.Request{})
		if err != nil {
			rt.failProse(gen, err)
			return
		}
		if !rt.pumpSegment(ctx, gen, st) {
			return // отмена/сбой уже обработаны внутри
		}
		if !rt.persistSegment(gen, seg.role, seg.speaker) {
			return
		}
	}
	rt.finishProse(gen)
}

// pumpSegment релеит дельты одного сегмента до EOF. false — генерация устарела,
// отменена или упала (в последнем случае failProse уже вызван). Закрывает поток.
func (rt *sessionRuntime) pumpSegment(ctx context.Context, gen int, st llm.Stream) bool {
	defer st.Close()
	ix := 0
	for {
		d, err := st.Recv()
		if d.Text != "" {
			if !rt.emitDelta(gen, d.Text, ix) {
				return false // суперсессия или отмена: замолкаем
			}
			ix++
		}
		if err == io.EOF {
			return true
		}
		if err != nil {
			rt.failProse(gen, err)
			return false
		}
		select {
		case <-ctx.Done():
			return false
		default:
		}
	}
}

// beginSegment открывает следующий сегмент: сбрасывает буфер, ставит роль/имя и
// шлёт OpStart. false — поколение устарело. Под собственным rt.mu.
func (rt *sessionRuntime) beginSegment(gen int, role, speaker string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if gen != rt.narrateGen {
		return false
	}
	rt.narration = nil
	rt.narrateRole = role
	rt.narrateSpeaker = speaker
	rt.broadcastLocked(rt.startFrameLocked(role, speaker))
	return true
}

// persistSegment кладёт завершённый сегмент в ленту (долговечная история).
// false — поколение устарело. Под собственным rt.mu.
func (rt *sessionRuntime) persistSegment(gen int, role, speaker string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if gen != rt.narrateGen {
		return false
	}
	if text := strings.Join(rt.narration, ""); text != "" {
		// Не канон — ошибку записи не роняем в игрока, но делаем слышимой.
		if err := rt.store.AppendTranscript(context.Background(), store.SessionID(rt.chatID),
			TranscriptEntry{Role: role, Text: text, Speaker: speaker}); err != nil {
			log.Printf("лента: сегмент прозы не записан: %v", err)
		}
	}
	return true
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

// finishProse завершает ход кадром done. Сегменты уже легли в ленту по мере
// завершения; здесь только снимается признак генерации и шлётся done.
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
// корректным session_state, просто без прозы (или без части) этого хода.
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

// kindOf переводит вид прозы презентации в вид Мастера.
func kindOf(k cli.ProseKind) master.Kind {
	switch k {
	case cli.ProsePlace:
		return master.KindPlace
	case cli.ProseBriefing:
		return master.KindBriefing
	case cli.ProseProbe:
		return master.KindProbe
	case cli.ProseReply:
		return master.KindReply
	default:
		return master.KindOutcome
	}
}
