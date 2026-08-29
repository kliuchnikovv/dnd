package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kliuchnikovv/dnd/auth"
)

// Server — HTTP-грань транспорта: health для Railway и старт сессии. Всё
// realtime уходит через WebSocket (следующая фаза); здесь только минимальный
// REST рядом с ним.
type Server struct {
	mgr     *Manager
	mux     *http.ServeMux
	auth    *auth.Service
	devAuth bool
}

// Option настраивает Server при создании (New). Опционально — сервер без
// опций работает как раньше, без auth-маршрутов.
type Option func(*Server)

// WithAuth подключает auth.Service и включает маршруты /auth/*, /me.
// devAuth разрешает POST /auth/dev (вход без Google — для локали и тестов).
func WithAuth(a *auth.Service, devAuth bool) Option {
	return func(s *Server) { s.auth = a; s.devAuth = devAuth }
}

// New собирает маршруты. Хендлер отдаётся через Handler(), а жизненный цикл
// http.Server (порт, graceful shutdown) держит cmd/server: пакет остаётся
// тестируемым через httptest без поднятия сокета.
func New(mgr *Manager, opts ...Option) *Server {
	s := &Server{mgr: mgr, mux: http.NewServeMux()}
	for _, opt := range opts {
		opt(s)
	}
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	// Путь совместим с клиентом nomi: GET /chat/ws?token=&chat_id=.
	s.mux.HandleFunc("/chat/ws", s.handleWS)
	if s.auth != nil {
		s.mux.HandleFunc("POST /sessions", s.requireAuth(s.handleCreateSession))
		s.mux.HandleFunc("POST /auth/google", s.handleGoogleLogin)
		s.mux.HandleFunc("POST /auth/refresh", s.handleRefresh)
		s.mux.HandleFunc("GET /me", s.requireAuth(s.handleMe))
		s.mux.HandleFunc("POST /auth/dev", s.handleDevLogin)
	} else {
		s.mux.HandleFunc("POST /sessions", s.handleCreateSession)
	}
	return s
}

// Handler — http.Handler со всеми маршрутами.
func (s *Server) Handler() http.Handler { return s.mux }

// AuthService строит auth.Service поверх хранилища менеджера. userStoreAdapter
// и m.store неэкспортируемы, поэтому cmd/server не может собрать сервис сам —
// этот конструктор остаётся единственным мостом наружу пакета.
func (m *Manager) AuthService(v auth.Verifier, secret string, now func() time.Time, newID func() string) *auth.Service {
	return auth.NewService(v, auth.NewTokens(secret, now), userStoreAdapter{st: m.store}, newID)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// createSessionRequest — тело POST /sessions. Seed необязателен: ноль
// означает «по умолчанию», а не «сессия без броска».
type createSessionRequest struct {
	Case string `json:"case"`
	Seed *int64 `json:"seed"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "тело запроса не разобрано")
		return
	}
	seed := int64(1)
	if req.Seed != nil {
		seed = *req.Seed
	}
	chatID, err := s.mgr.Create(req.Case, seed, userIDFrom(r.Context()))
	if err != nil {
		// Единственная ошибка Create — про дело: не нашли или не разобрали.
		// Это ошибка запроса, не сервера.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"chat_id": chatID})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
