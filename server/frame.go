package server

import "encoding/json"

// Frame — единица WS-протокола, побайтово совместимая с клиентом nomi
// (genie-api/front/src/screens/main/models.tsx: type Frame). Форма НЕ
// расширяется без сверки с клиентом: имена и типы полей — контракт провода.
//
// Payload — сырой JSON: на входе разворачивается по op в конкретную форму, на
// выходе собирается из доменной структуры. Error — свободная форма nomi.
type Frame struct {
	ID      int             `json:"id"`
	ChatID  string          `json:"chat_id"`
	Channel string          `json:"channel"`
	Kind    string          `json:"kind"`
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   any             `json:"error,omitempty"`
}

// Каналы.
const (
	ChannelChat = "chat" // сессия игрока
	ChannelUser = "user" // глобальный пуш событий мира (заложен, пуст в MVP)
)

// Виды.
const (
	KindData   = "data"
	KindMeta   = "meta"
	KindSignal = "signal"
	KindError  = "error"
)

// Операции.
const (
	OpMessage      = "message"       // ввод игрока / дельта прозы
	OpPing         = "ping"          // heartbeat
	OpStart        = "start"         // генерация прозы началась (лоадер у клиента)
	OpDone         = "done"          // генерация хода завершена
	OpHistory      = "history"       // недавнее при возобновлении
	OpTranscript   = "transcript"    // полная история ходов при возобновлении
	OpSessionState = "session_state" // полный TurnView текущего хода
	OpStop         = "stop"          // отмена генерации
	OpTools        = "tools"         // список инструментов (заложено nomi)
)

// inputPayload — тело op:"message" от игрока. Token разворачивается сервером в
// интент показанного набора; Text — номер варианта (свободный NL — фаза LLM).
// Клиент интентов не сочиняет: он лишь ссылается на предложенное.
type inputPayload struct {
	Token string `json:"token,omitempty"`
	Text  string `json:"text,omitempty"`
}

// textDelta — тело дельты прозы, форма nomi: payload {type:"text", delta, ix}.
type textDelta struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
	IX    int    `json:"ix"`
}

// newFrame собирает исходящий кадр, маршаля payload в сырой JSON. Ошибка
// маршалинга доменной структуры — это баг сервера, не провода: payload тогда
// пуст, а вид кадра сохраняется.
func newFrame(id int, chatID, channel, kind, op string, payload any) Frame {
	f := Frame{ID: id, ChatID: chatID, Channel: channel, Kind: kind, Op: op}
	if payload != nil {
		if raw, err := json.Marshal(payload); err == nil {
			f.Payload = raw
		}
	}
	return f
}

// errorFrame — кадр сбоя. Невалидный ход отвечает им, а не разрывом: сессия
// живёт, игрок пробует другой вариант.
func errorFrame(id int, chatID string, msg string) Frame {
	return Frame{
		ID:      id,
		ChatID:  chatID,
		Channel: ChannelChat,
		Kind:    KindError,
		Op:      OpMessage,
		Error:   map[string]string{"message": msg},
	}
}
