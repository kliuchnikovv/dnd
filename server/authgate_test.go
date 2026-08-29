package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/kliuchnikovv/dnd/auth"
)

// createSessionAs делает POST /sessions с Bearer-токеном и возвращает chat_id.
func createSessionAs(t *testing.T, srv *Server, token, caseName string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"case":"`+caseName+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("создание сессии: код %d, тело %q", rec.Code, rec.Body.String())
	}
	var out struct {
		ChatID string `json:"chat_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("ответ не JSON: %v (%q)", err, rec.Body.String())
	}
	return out.ChatID
}

// POST /sessions без Bearer на auth-сервере — 401: гейт стоит перед созданием
// сессии, не только перед /me.
func TestSessionsRequiresAuth(t *testing.T) {
	srv, _ := testServerWithAuth(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"case":"harbour"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена ждали 401, получили %d", rec.Code)
	}
}

// Сессия принадлежит создателю: чужой валидный токен на её chat_id получает
// 403, а не доступ к чужой игре.
func TestWSRejectsForeignChatID(t *testing.T) {
	srv, svc := testServerWithAuth(t)

	// пользователь A создаёт сессию
	a, err := svc.LoginAs(context.Background(), auth.Identity{Sub: "gA", Email: "a@x"})
	if err != nil {
		t.Fatal(err)
	}
	idA := createSessionAs(t, srv, a.AccessToken, "harbour")

	// пользователь B пытается подключиться к сессии A
	b, err := svc.LoginAs(context.Background(), auth.Identity{Sub: "gB", Email: "b@x"})
	if err != nil {
		t.Fatal(err)
	}

	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	url := "ws" + strings.TrimPrefix(hs.URL, "http") + "/chat/ws?token=" + b.AccessToken + "&chat_id=" + idA
	_, resp, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatal("чужой chat_id принят")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("ждали 403, получили %v", resp)
	}
}

// Владелец может подключиться к своей же сессии — гейт не режет легитимный
// путь.
func TestWSAllowsOwnChatID(t *testing.T) {
	srv, svc := testServerWithAuth(t)

	a, err := svc.LoginAs(context.Background(), auth.Identity{Sub: "gOwner", Email: "o@x"})
	if err != nil {
		t.Fatal(err)
	}
	idA := createSessionAs(t, srv, a.AccessToken, "harbour")

	conn, done := wsDial(t, srv, idA, a.AccessToken)
	defer done()

	decodeView(t, readFrame(t, conn))
}

// GET /sessions отдаёт только сессии вызывающего, не чужие.
func TestListSessionsReturnsOnlyOwn(t *testing.T) {
	srv, svc := testServerWithAuth(t)
	a, _ := svc.LoginAs(context.Background(), auth.Identity{Sub: "gA", Email: "a@x"})
	b, _ := svc.LoginAs(context.Background(), auth.Identity{Sub: "gB", Email: "b@x"})
	idA := createSessionAs(t, srv, a.AccessToken, "harbour")
	createSessionAs(t, srv, b.AccessToken, "harbour")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+a.AccessToken)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("код %d тело %q", rec.Code, rec.Body.String())
	}
	var out []struct {
		ChatID string `json:"chat_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].ChatID != idA {
		t.Fatalf("ждали только сессию A (%s), получили %+v", idA, out)
	}
}

// GET /sessions без Bearer — 401.
func TestListSessionsRequiresAuth(t *testing.T) {
	srv, _ := testServerWithAuth(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/sessions", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена ждали 401, получили %d", rec.Code)
	}
}
