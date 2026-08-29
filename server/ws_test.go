package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/kliuchnikovv/dnd/view"
)

// wsDial поднимает httptest-сервер и подключается к chat-каналу сессии.
func wsDial(t *testing.T, srv *Server, chatID string) (*websocket.Conn, func()) {
	t.Helper()
	hs := httptest.NewServer(srv.Handler())
	url := "ws" + strings.TrimPrefix(hs.URL, "http") +
		"/chat/ws?token=dev&chat_id=" + chatID
	conn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		hs.Close()
		t.Fatalf("dial: %v", err)
	}
	return conn, func() {
		conn.Close(websocket.StatusNormalClosure, "")
		hs.Close()
	}
}

func readFrame(t *testing.T, conn *websocket.Conn) Frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var f Frame
	if err := wsjson.Read(ctx, conn, &f); err != nil {
		t.Fatalf("чтение кадра: %v", err)
	}
	return f
}

func decodeView(t *testing.T, f Frame) view.TurnView {
	t.Helper()
	if f.Op != OpSessionState {
		t.Fatalf("ждали session_state, получили op=%q kind=%q", f.Op, f.Kind)
	}
	var tv view.TurnView
	if err := json.Unmarshal(f.Payload, &tv); err != nil {
		t.Fatalf("session_state не TurnView: %v", err)
	}
	return tv
}

// На (ре)коннекте первым кадром приходит session_state текущего хода.
func TestWSConnectSendsSessionState(t *testing.T) {
	srv := New(NewManager(casesRoot))
	id, err := srv.mgr.Create("harbour", 1)
	if err != nil {
		t.Fatal(err)
	}
	conn, done := wsDial(t, srv, id)
	defer done()

	tv := decodeView(t, readFrame(t, conn))
	if len(tv.Options) == 0 {
		t.Fatalf("стартовый session_state без вариантов — играть нечем")
	}
}

// Полный ход «Гавани» через WS: выбор варианта по токену даёт новый
// session_state с корректным TurnView.
func TestWSTurnByToken(t *testing.T) {
	srv := New(NewManager(casesRoot))
	id, _ := srv.mgr.Create("harbour", 1)
	conn, done := wsDial(t, srv, id)
	defer done()

	start := decodeView(t, readFrame(t, conn))
	token := start.Options[0].Token
	if token == "" {
		t.Fatalf("у первого варианта нет токена")
	}

	writeInput(t, conn, 1, inputPayload{Token: token})
	after := decodeView(t, readFrame(t, conn))
	if after.Version == 0 {
		t.Fatalf("TurnView после хода без версии формы")
	}
}

// Токен и номер — один путь: выбор первого варианта номером «1» и его же
// токеном дают одинаковый исход.
func TestWSTokenAndNumberSamePath(t *testing.T) {
	srv := New(NewManager(casesRoot))

	byToken := runFirstTurn(t, srv, func(o view.Option) inputPayload {
		return inputPayload{Token: o.Token}
	})
	byNumber := runFirstTurn(t, srv, func(view.Option) inputPayload {
		return inputPayload{Text: "1"}
	})

	if byToken.Scene.Title != byNumber.Scene.Title {
		t.Fatalf("токен и номер разошлись: %q vs %q",
			byToken.Scene.Title, byNumber.Scene.Title)
	}
}

// runFirstTurn проигрывает один ход на свежей сессии, выбирая первый вариант
// способом choose, и возвращает получившийся TurnView.
func runFirstTurn(t *testing.T, srv *Server, choose func(view.Option) inputPayload) view.TurnView {
	t.Helper()
	id, _ := srv.mgr.Create("harbour", 1)
	conn, done := wsDial(t, srv, id)
	defer done()
	start := decodeView(t, readFrame(t, conn))
	writeInput(t, conn, 1, choose(start.Options[0]))
	return decodeView(t, readFrame(t, conn))
}

// Невалидный ввод отвечает error-кадром, а не разрывом: следующий валидный ход
// на том же сокете проходит.
func TestWSInvalidInputKeepsConnection(t *testing.T) {
	srv := New(NewManager(casesRoot))
	id, _ := srv.mgr.Create("harbour", 1)
	conn, done := wsDial(t, srv, id)
	defer done()

	start := decodeView(t, readFrame(t, conn))

	writeInput(t, conn, 1, inputPayload{Token: "нет-такого-токена"})
	bad := readFrame(t, conn)
	if bad.Kind != KindError {
		t.Fatalf("ждали error-кадр, получили kind=%q op=%q", bad.Kind, bad.Op)
	}

	// Сокет жив: валидный ход проходит.
	writeInput(t, conn, 2, inputPayload{Token: start.Options[0].Token})
	decodeView(t, readFrame(t, conn))
}

// Повтор кадра с тем же id не применяет ход дважды: сервер лишь пере-отдаёт
// состояние (идемпотентность доставки).
func TestWSIdempotentReplay(t *testing.T) {
	srv := New(NewManager(casesRoot))
	id, _ := srv.mgr.Create("harbour", 1)
	conn, done := wsDial(t, srv, id)
	defer done()

	start := decodeView(t, readFrame(t, conn))
	rt, _ := srv.mgr.Get(id)

	writeInput(t, conn, 7, inputPayload{Token: start.Options[0].Token})
	decodeView(t, readFrame(t, conn))
	rt.mu.Lock()
	appliedOnce := rt.lastAppliedID
	rt.mu.Unlock()

	// Тот же id ещё раз: состояние приходит, но ход не применяется.
	writeInput(t, conn, 7, inputPayload{Token: start.Options[0].Token})
	decodeView(t, readFrame(t, conn))
	rt.mu.Lock()
	appliedTwice := rt.lastAppliedID
	rt.mu.Unlock()

	if appliedOnce != 7 || appliedTwice != 7 {
		t.Fatalf("идемпотентность нарушена: lastApplied %d → %d", appliedOnce, appliedTwice)
	}
}

// Пустой token — 401 ещё до апгрейда.
func TestWSRequiresToken(t *testing.T) {
	srv := New(NewManager(casesRoot))
	id, _ := srv.mgr.Create("harbour", 1)
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	url := "ws" + strings.TrimPrefix(hs.URL, "http") + "/chat/ws?chat_id=" + id
	_, resp, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatalf("ждали отказ без токена")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ждали 401, получили %v", resp)
	}
}

// Неизвестный chat_id — 404.
func TestWSUnknownSession(t *testing.T) {
	srv := New(NewManager(casesRoot))
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	url := "ws" + strings.TrimPrefix(hs.URL, "http") + "/chat/ws?token=dev&chat_id=нет"
	_, resp, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatalf("ждали отказ на неизвестной сессии")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("ждали 404, получили %v", resp)
	}
}

func writeInput(t *testing.T, conn *websocket.Conn, id int, in inputPayload) {
	t.Helper()
	raw, _ := json.Marshal(in)
	f := Frame{
		ID:      id,
		Channel: ChannelChat,
		Kind:    KindData,
		Op:      OpMessage,
		Payload: raw,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, conn, f); err != nil {
		t.Fatalf("запись ввода: %v", err)
	}
}
