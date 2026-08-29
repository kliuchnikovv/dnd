package server

import (
	"encoding/json"
	"net/http"
)

// Server — HTTP-грань транспорта: health для Railway и старт сессии. Всё
// realtime уходит через WebSocket (следующая фаза); здесь только минимальный
// REST рядом с ним.
type Server struct {
	mgr *Manager
	mux *http.ServeMux
}

// New собирает маршруты. Хендлер отдаётся через Handler(), а жизненный цикл
// http.Server (порт, graceful shutdown) держит cmd/server: пакет остаётся
// тестируемым через httptest без поднятия сокета.
func New(mgr *Manager) *Server {
	s := &Server{mgr: mgr, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /sessions", s.handleCreateSession)
	return s
}

// Handler — http.Handler со всеми маршрутами.
func (s *Server) Handler() http.Handler { return s.mux }

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
	chatID, err := s.mgr.Create(req.Case, seed)
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
