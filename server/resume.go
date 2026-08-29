package server

import "strings"

// resumeBuffer — ёмкость очереди свежего сокета: она обязана вместить залповую
// отдачу при (ре)коннекте (session_state + буфер прозы хода) до того, как её
// начнёт вычитывать насос. Проза ограничена MaxTokens (~900), поэтому дельт на
// ход заведомо меньше; запас взят с большим гандикапом.
const resumeBuffer = 4096

// attach регистрирует сокет и сразу отдаёт ему состояние — под ОДНИМ rt.mu,
// чтобы возобновление не разъехалось с живой рассылкой:
//
//   - session_state — где игра сейчас;
//   - если проза хода В ПОЛЁТЕ: буферные дельты (догон), после чего живые
//     дельты доедут той же рассылкой — сокет уже в подписчиках, ix продолжится
//     без разрыва и без дублей (горутина шлёт только НОВЫЕ дельты);
//   - если проза хода ЗАВЕРШЕНА: один кадр history с готовым текстом — заново
//     печатать её дельтами незачем, ход уже кончился.
//
// Всё это кладётся в очередь сокета до старта насоса: буфер заведомо вмещает
// залп, поэтому запись под mu не блокирует.
func (rt *sessionRuntime) attach() *subscriber {
	sub := &subscriber{out: make(chan Frame, resumeBuffer)}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.subs[sub] = struct{}{}

	sub.out <- rt.snapshotViewLocked()

	switch {
	case rt.narrating:
		// Догоняем полёт: то, что уже сгенерилось. Остаток доедет рассылкой.
		for ix, d := range rt.narration {
			sub.out <- rt.deltaFrameLocked(d, ix)
		}
	case len(rt.narration) > 0:
		// Ход завершён: отдаём прозу одним кадром history.
		sub.out <- rt.historyFrameLocked()
	}
	return sub
}

// deltaFrameLocked собирает кадр дельты прозы. Под rt.mu.
func (rt *sessionRuntime) deltaFrameLocked(text string, ix int) Frame {
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpMessage, textDelta{Type: "text", Delta: text, IX: ix})
}

// historyFrameLocked собирает кадр history с готовой прозой последнего хода.
// Под rt.mu.
func (rt *sessionRuntime) historyFrameLocked() Frame {
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpHistory, historyPayload{Type: "narration", Text: strings.Join(rt.narration, "")})
}

// historyPayload — тело кадра history: готовая проза недавнего хода.
type historyPayload struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
