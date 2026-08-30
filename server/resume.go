package server

import (
	"context"
	"log"

	"github.com/kliuchnikovv/dnd/store"
)

// resumeBuffer — ёмкость очереди свежего сокета: она обязана вместить залповую
// отдачу при (ре)коннекте (session_state + буфер прозы хода) до того, как её
// начнёт вычитывать насос. Проза ограничена MaxTokens (~900), поэтому дельт на
// ход заведомо меньше; запас взят с большим гандикапом.
const resumeBuffer = 4096

// attach регистрирует сокет и сразу отдаёт ему состояние — под ОДНИМ rt.mu,
// чтобы возобновление не разъехалось с живой рассылкой:
//
//   - session_state — где игра сейчас;
//   - transcript — вся долговечная история партии (проза прошлых ходов и эхо
//     действий); переживает реконнект и рестарт. Проза ТЕКУЩЕГО хода в ленту
//     ещё не легла (её пишет finishProse), поэтому дублей с полётом нет;
//   - если проза хода В ПОЛЁТЕ: сигнал «генерю» (лоадер) + буферные дельты
//     (догон), после чего живые дельты доедут той же рассылкой — сокет уже в
//     подписчиках, ix продолжится без разрыва и без дублей.
//
// Всё это кладётся в очередь сокета до старта насоса: буфер заведомо вмещает
// залп, поэтому запись под mu не блокирует.
func (rt *sessionRuntime) attach() *subscriber {
	sub := &subscriber{out: make(chan Frame, resumeBuffer)}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.subs[sub] = struct{}{}

	sub.out <- rt.snapshotViewLocked()

	// История: не канон — при ошибке чтения продолжаем без неё, но слышимо.
	entries, err := rt.store.Transcript(context.Background(), store.SessionID(rt.chatID))
	if err != nil {
		log.Printf("лента: чтение при коннекте не удалось: %v", err)
	} else if len(entries) > 0 {
		sub.out <- rt.transcriptFrameLocked(entries)
	}

	if rt.narrating {
		// Проза в полёте: сперва сигнал «генерю» с ролью текущего сегмента
		// (лоадер/нужный блок), затем догон уже сгенерённого. Остаток доедет
		// живой рассылкой. Завершённые сегменты уже в transcript — дублей нет.
		sub.out <- rt.startFrameLocked(rt.narrateRole, rt.narrateSpeaker)
		for ix, d := range rt.narration {
			sub.out <- rt.deltaFrameLocked(d, ix)
		}
	}
	return sub
}

// transcriptFrameLocked собирает кадр всей истории партии. Под rt.mu.
func (rt *sessionRuntime) transcriptFrameLocked(entries []TranscriptEntry) Frame {
	items := make([]transcriptItem, len(entries))
	for i, e := range entries {
		items[i] = transcriptItem{Role: e.Role, Text: e.Text, Speaker: e.Speaker}
	}
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpTranscript, transcriptPayload{Type: "transcript", Entries: items})
}

// transcriptPayload — тело кадра transcript: вся история партии по порядку.
type transcriptPayload struct {
	Type    string           `json:"type"`
	Entries []transcriptItem `json:"entries"`
}

type transcriptItem struct {
	Role    string `json:"role"`
	Text    string `json:"text"`
	Speaker string `json:"speaker,omitempty"`
}

// deltaFrameLocked собирает кадр дельты прозы. Под rt.mu.
func (rt *sessionRuntime) deltaFrameLocked(text string, ix int) Frame {
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpMessage, textDelta{Type: "text", Delta: text, IX: ix})
}

