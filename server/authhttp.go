package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/kliuchnikovv/dnd/auth"
)

// userStoreAdapter приводит серверный Store к auth.UserStore (auth не знает о
// server). Поля UserRecord и auth.User совпадают по именам.
type userStoreAdapter struct{ st Store }

func (a userStoreAdapter) UpsertUser(ctx context.Context, u auth.User) error {
	return a.st.UpsertUser(ctx, UserRecord(u))
}
func (a userStoreAdapter) UserByGoogleSub(ctx context.Context, sub string) (auth.User, bool, error) {
	r, ok, err := a.st.UserByGoogleSub(ctx, sub)
	return auth.User(r), ok, err
}
func (a userStoreAdapter) UserByID(ctx context.Context, id string) (auth.User, bool, error) {
	r, ok, err := a.st.UserByID(ctx, id)
	return auth.User(r), ok, err
}
func (a userStoreAdapter) SaveRefresh(ctx context.Context, hash, userID string, exp time.Time) error {
	return a.st.SaveRefresh(ctx, hash, userID, exp)
}
func (a userStoreAdapter) RefreshOwner(ctx context.Context, hash string) (string, bool, error) {
	return a.st.RefreshOwner(ctx, hash)
}
func (a userStoreAdapter) DeleteRefresh(ctx context.Context, hash string) error {
	return a.st.DeleteRefresh(ctx, hash)
}
func (a userStoreAdapter) ClaimRefresh(ctx context.Context, hash string) (string, bool, error) {
	return a.st.ClaimRefresh(ctx, hash)
}

func (s *Server) handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IDToken == "" {
		writeError(w, http.StatusBadRequest, "нужен id_token")
		return
	}
	sess, err := s.auth.GoogleLogin(r.Context(), req.IDToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "вход отклонён")
		return
	}
	writeSession(w, sess)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "нужен refresh_token")
		return
	}
	sess, err := s.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "refresh отклонён")
		return
	}
	writeSession(w, sess)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	u, ok, err := s.auth.Me(r.Context(), uid)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "нет пользователя")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// handleDevLogin — вход без Google для локали/тестов. Работает только при devAuth.
func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !s.devAuth {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "нужен email")
		return
	}
	sess, err := s.auth.LoginAs(r.Context(), auth.Identity{Sub: "dev:" + req.Email, Email: req.Email, Name: req.Email})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSession(w, sess)
}

// handleListSessions — GET /sessions: список сессий владельца (для экрана
// выбора сессии на клиенте). POST /sessions остаётся для создания новой игры.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	recs, err := s.mgr.store.SessionsByUser(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать сессии")
		return
	}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, map[string]any{
			"chat_id": r.ChatID, "case_id": r.CaseID,
			"seed": r.Seed, "created_at": r.CreatedAt.Unix(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func writeSession(w http.ResponseWriter, s auth.Session) {
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  s.AccessToken,
		"refresh_token": s.RefreshToken,
		"expires_at":    s.ExpiresAt.Unix(),
		"user":          s.User,
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// userIDCtxKey — тип ключа контекста для userID; не экспортируется, чтобы
// исключить коллизии с другими пакетами.
type userIDCtxKey struct{}

func userIDFrom(ctx context.Context) string {
	uid, _ := ctx.Value(userIDCtxKey{}).(string)
	return uid
}

// requireAuth — мидлварь: читает Bearer-токен, проверяет его через
// auth.Service.Verify и кладёт userID в контекст. Без валидного токена — 401.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "нужен токен")
			return
		}
		uid, err := s.auth.Verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "токен недействителен")
			return
		}
		ctx := context.WithValue(r.Context(), userIDCtxKey{}, uid)
		next(w, r.WithContext(ctx))
	}
}
