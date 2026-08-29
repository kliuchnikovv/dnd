package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// handleWS — единственная точка realtime. Апгрейд, канал по query (как nomi:
// token в query, chat_id — сессия), затем: отдать текущий session_state и
// слушать ввод игрока. Генерация не привязана к сокету — сессия рассылает
// session_state всем подписчикам, поэтому реконнект и второе устройство видят
// один и тот же ход.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Анонимный токен устройства: любой непустой принимается, пустой — 401.
	// Проверка подписи и Apple-auth — позже; шов уже стоит.
	if q.Get("token") == "" {
		http.Error(w, "нужен token", http.StatusUnauthorized)
		return
	}

	channel := q.Get("channel")
	if channel == "" {
		channel = ChannelChat
	}
	if channel == ChannelUser {
		// Канал событий мира заложен, но пуст в MVP: держим сокет открытым,
		// ничего не шлём. Наполнится с мультиплеером.
		s.serveUser(w, r)
		return
	}

	chatID := q.Get("chat_id")
	rt, ok := s.mgr.Get(chatID)
	if !ok {
		http.Error(w, "нет такой сессии", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	sub := rt.subscribe()
	defer rt.unsubscribe(sub)

	// Первое, что видит (ре)подключившийся, — где игра сейчас. В фазе 5 сюда
	// добавится history и досстрим прозы в полёте.
	sub.out <- rt.snapshotView()

	// Единственный писатель сокета — насос: coder/websocket запрещает
	// конкурентную запись, поэтому все кадры (session_state, error, pong)
	// идут через sub.out.
	go writePump(ctx, cancel, conn, sub)

	readLoop(ctx, conn, rt, sub)
}

// serveUser держит канал user открытым до закрытия клиентом. MVP не шлёт по
// нему ничего; форма готова под пуш событий мира.
func (s *Server) serveUser(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()
	for {
		var f Frame
		if err := wsjson.Read(ctx, conn, &f); err != nil {
			return
		}
	}
}

// writePump — единственный писатель сокета. Ошибка записи рвёт соединение через
// cancel: читающий цикл увидит закрытый контекст и выйдет.
func writePump(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, sub *subscriber) {
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-sub.out:
			if err := wsjson.Write(ctx, conn, f); err != nil {
				cancel()
				return
			}
		}
	}
}

// readLoop разбирает ввод игрока. Невалидный ход — error-кадр, а не разрыв:
// сессия живёт, игрок пробует другой вариант.
func readLoop(ctx context.Context, conn *websocket.Conn, rt *sessionRuntime, sub *subscriber) {
	for {
		var f Frame
		if err := wsjson.Read(ctx, conn, &f); err != nil {
			return
		}
		switch f.Op {
		case OpMessage:
			var in inputPayload
			if len(f.Payload) > 0 {
				_ = json.Unmarshal(f.Payload, &in)
			}
			ok, msg := rt.applyInput(ctx, f.ID, in)
			if !ok {
				send(ctx, sub, errorFrame(f.ID, rt.chatID, msg))
			}
		case OpPing:
			rt.mu.Lock()
			id := rt.nextOutIDLocked()
			rt.mu.Unlock()
			send(ctx, sub, newFrame(id, rt.chatID, ChannelChat, KindSignal, OpPing, nil))
		case OpStop:
			// Отмена генерации прозы текущего хода. Механику не трогает: она
			// уже применена ядром и ушла в session_state.
			rt.stopProse()
		}
	}
}

// send кладёт кадр в очередь подписчика, не блокируя на закрытии.
func send(ctx context.Context, sub *subscriber, f Frame) {
	select {
	case sub.out <- f:
	case <-ctx.Done():
	}
}
