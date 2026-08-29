package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// casesRoot — дела лежат на уровень выше пакета server.
const casesRoot = "../cases"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(NewManager(casesRoot))
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: код %d, тело %q", rec.Code, rec.Body.String())
	}
}

func TestCreateSession(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"case":"harbour","seed":1}`)
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sessions", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("создание сессии: код %d, тело %q", rec.Code, rec.Body.String())
	}
	var out struct {
		ChatID string `json:"chat_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("ответ не JSON: %v (%q)", err, rec.Body.String())
	}
	if !strings.HasPrefix(out.ChatID, "harbour@") {
		t.Fatalf("chat_id %q не начинается с идентификатора дела", out.ChatID)
	}
	// Сессия должна быть доступна менеджеру по возвращённому chat_id.
	if _, ok := srv.mgr.Get(out.ChatID); !ok {
		t.Fatalf("сессия %q не найдена в менеджере", out.ChatID)
	}
}

// Seed необязателен: без него ход по умолчанию, а не отказ.
func TestCreateSessionDefaultSeed(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sessions",
		strings.NewReader(`{"case":"harbour"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("сессия без seed: код %d, тело %q", rec.Code, rec.Body.String())
	}
}

func TestCreateSessionUnknownCase(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sessions",
		strings.NewReader(`{"case":"нет-такого-дела"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("неизвестное дело: ждали 400, получили %d", rec.Code)
	}
}

func TestCreateSessionMissingCase(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sessions",
		strings.NewReader(`{"seed":1}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("пустое дело: ждали 400, получили %d", rec.Code)
	}
}

func TestCreateSessionBadJSON(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/sessions",
		strings.NewReader(`не json`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("битый JSON: ждали 400, получили %d", rec.Code)
	}
}
