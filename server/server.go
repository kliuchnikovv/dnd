package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kliuchnikovv/dnd/auth"
	"github.com/kliuchnikovv/dnd/core"
)

// Server — HTTP-грань транспорта: health для Railway и старт сессии. Всё
// realtime уходит через WebSocket (следующая фаза); здесь только минимальный
// REST рядом с ним.
type Server struct {
	mgr     *Manager
	mux     *http.ServeMux
	auth    *auth.Service
	devAuth bool
	catalog *CaseCatalog
	// characters — сервер-уровневое хранилище персонажей (MVP: in-memory, см.
	// CharacterStore). nil — POST /sessions работает по легаси-пути без
	// character_id (только case).
	characters *CharacterStore
}

// Option настраивает Server при создании (New). Опционально — сервер без
// опций работает как раньше, без auth-маршрутов.
type Option func(*Server)

// WithAuth подключает auth.Service и включает маршруты /auth/*, /me.
// devAuth разрешает POST /auth/dev (вход без Google — для локали и тестов).
func WithAuth(a *auth.Service, devAuth bool) Option {
	return func(s *Server) { s.auth = a; s.devAuth = devAuth }
}

// WithCatalog подключает каталог дел и включает GET /cases. Без опции
// эндпоинт не регистрируется — сервер работает как раньше.
func WithCatalog(c *CaseCatalog) Option {
	return func(s *Server) { s.catalog = c }
}

// WithCharacters подключает хранилище персонажей: POST /sessions принимает
// character_id и сверяет ruleset персонажа с ruleset дела (см.
// handleCreateSession). character_id обязателен — character в case.json
// не живёт, взять лист персонажа неоткуда, кроме как из этого хранилища.
func WithCharacters(c *CharacterStore) Option {
	return func(s *Server) { s.characters = c }
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
		s.mux.HandleFunc("GET /sessions", s.requireAuth(s.handleListSessions))
		s.mux.HandleFunc("POST /vignette/from-theme", s.requireAuth(s.handleCreateVignetteFromTheme))
		s.mux.HandleFunc("POST /auth/google", s.handleGoogleLogin)
		s.mux.HandleFunc("POST /auth/refresh", s.handleRefresh)
		s.mux.HandleFunc("GET /me", s.requireAuth(s.handleMe))
		s.mux.HandleFunc("POST /auth/dev", s.handleDevLogin)
	} else {
		s.mux.HandleFunc("POST /sessions", s.handleCreateSession)
		s.mux.HandleFunc("POST /vignette/from-theme", s.handleCreateVignetteFromTheme)
	}
	if s.catalog != nil {
		s.mux.HandleFunc("GET /cases", s.catalog.HandleList)
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
// означает «по умолчанию», а не «сессия без броска». CaseID — новое имя поля
// по контракту ({case_id, character_id}); Case остаётся для обратной
// совместимости со старым клиентом ({case}) — см. caseName().
// CharacterID обязателен: character в case.json больше не живёт, и без него
// неоткуда взять лист персонажа и систему правил для сессии.
type createSessionRequest struct {
	Case        string `json:"case"`
	CaseID      string `json:"case_id"`
	CharacterID string `json:"character_id"`
	Seed        *int64 `json:"seed"`
}

// caseName — имя дела из запроса: case_id приоритетнее старого case.
func (req createSessionRequest) caseName() string {
	if req.CaseID != "" {
		return req.CaseID
	}
	return req.Case
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
	caseName := req.caseName()
	userID := userIDFrom(r.Context())

	if req.CharacterID == "" {
		writeError(w, http.StatusBadRequest, "character_id обязателен")
		return
	}
	if s.characters == nil {
		writeError(w, http.StatusBadRequest, "хранилище персонажей не подключено")
		return
	}

	character, ok := s.characters.ByID(req.CharacterID)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("персонаж %q не найден", req.CharacterID))
		return
	}
	caseRules, err := s.mgr.CaseRulesKind(caseName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Пустой Ruleset у персонажа — легаси-запись до character-owned ruleset,
	// читается как threshold (см. store.Character.Ruleset).
	characterRules := character.Ruleset
	if characterRules == "" {
		characterRules = string(core.RulesetThreshold)
	}
	if characterRules != caseRules {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"ruleset несовместим: персонаж %q — %q, дело %q — %q",
			req.CharacterID, characterRules, caseName, caseRules,
		))
		return
	}
	// Виньетка-трек (ADR-0009): у неё свой движок, не core.Game, поэтому ветка
	// уходит ДО LookupRuleset (в реестре core.RuleSystem виньетки нет и не будет).
	// Сцену читаем из scene.json дела; персонаж — гейт (его лист виньетке не нужен).
	if caseRules == VignetteRulesKind {
		chatID, err := s.mgr.CreateVignetteFromCase(caseName, seed, userID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"chat_id": chatID})
		return
	}

	rules, ok := core.LookupRuleset(core.RulesetKind(characterRules))
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("неизвестная система правил %q", characterRules))
		return
	}

	chatID, err := s.mgr.CreateWithRuleset(caseName, seed, userID, rules, character)
	if err != nil {
		// Единственная ошибка Create — про дело: не нашли или не разобрали.
		// Это ошибка запроса, не сервера.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"chat_id": chatID})
}

// createVignetteFromThemeRequest — тело POST /vignette/from-theme. Theme
// обязательно (без темы генератор не идёт); Mode необязателен (по умолчанию
// "hold" — соответствует cmd/vignette); Seed необязателен (по умолчанию 1 —
// как у POST /sessions).
type createVignetteFromThemeRequest struct {
	Theme string `json:"theme"`
	Mode  string `json:"mode"`
	Seed  *int64 `json:"seed"`
}

// handleCreateVignetteFromTheme — продуктовый эндпоинт core-генератора: тема →
// SceneSpec → CreateVignette. Виньетка — первый потребитель. Если генератор
// не подключён (офлайн-сервер без ключа) — 501: путь есть, но живого
// scenegen нет. Игрок не привязан к персонажу: у виньетки лист не читается,
// character_id гейт не требуется.
func (s *Server) handleCreateVignetteFromTheme(w http.ResponseWriter, r *http.Request) {
	if !s.mgr.SceneGeneratorAvailable() {
		writeError(w, http.StatusNotImplemented, "scenegen не подключён (сервер без ключа)")
		return
	}
	var req createVignetteFromThemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "тело запроса не разобрано")
		return
	}
	if req.Theme == "" {
		writeError(w, http.StatusBadRequest, "theme обязателен")
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = "hold"
	}
	seed := int64(1)
	if req.Seed != nil {
		seed = *req.Seed
	}
	userID := userIDFrom(r.Context())
	chatID, err := s.mgr.CreateVignetteFromTheme(r.Context(), req.Theme, mode, seed, userID)
	if err != nil {
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
