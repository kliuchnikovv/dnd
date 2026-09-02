package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/auth"
	"github.com/kliuchnikovv/dnd/store"
)

// testCharacterID — персонаж-threshold, которым тесты этого файла заходят в
// "harbour" (тоже threshold). Character в case.json больше не живёт: без
// character_id серверу неоткуда взять лист персонажа (см. server.go).
const testCharacterID = "chr_test"

func testServerWithAuth(t *testing.T) (*Server, *auth.Service) {
	t.Helper()
	mgr := NewManager(casesRoot)
	adapter := userStoreAdapter{st: mgr.store}
	seq := 0
	svc := auth.NewService(&auth.FakeVerifier{ID: auth.Identity{Sub: "g1", Email: "a@b.c", Name: "Ann"}},
		auth.NewTokens("secret", nil), adapter, func() string { seq++; return "u" + string(rune('0'+seq)) })
	cs := NewCharacterStore()
	cs.Save(&store.Character{ID: testCharacterID, Ruleset: "threshold"})
	srv := New(mgr, WithAuth(svc, true), WithCharacters(cs)) // devAuth=true
	return srv, svc
}

func TestGoogleLoginEndpoint(t *testing.T) {
	srv, _ := testServerWithAuth(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/auth/google",
		strings.NewReader(`{"id_token":"x"}`)))
	if rec.Code != 200 {
		t.Fatalf("код %d тело %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		User    struct{ Email string } `json:"user"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Access == "" || out.Refresh == "" || out.User.Email != "a@b.c" {
		t.Fatalf("ответ: %+v", out)
	}
}

func TestDevLoginBlockedWithoutFlag(t *testing.T) {
	mgr := NewManager(casesRoot)
	svc := auth.NewService(&auth.FakeVerifier{}, auth.NewTokens("s", nil), userStoreAdapter{st: mgr.store}, func() string { return "u" })
	srv := New(mgr, WithAuth(svc, false)) // devAuth=false
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/auth/dev", strings.NewReader(`{"email":"a@b.c"}`)))
	if rec.Code != 404 {
		t.Fatalf("dev-login без флага: код %d, ждали 404", rec.Code)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	srv, _ := testServerWithAuth(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/me", nil))
	if rec.Code != 401 {
		t.Fatalf("/me без токена: код %d, ждали 401", rec.Code)
	}
}

func TestMeWithValidToken(t *testing.T) {
	srv, _ := testServerWithAuth(t)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/auth/google", strings.NewReader(`{"id_token":"x"}`)))
	var login struct {
		Access string `json:"access_token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &login)

	rec2 := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Authorization", "Bearer "+login.Access)
	srv.Handler().ServeHTTP(rec2, req)
	if rec2.Code != 200 {
		t.Fatalf("/me с валидным токеном: код %d тело %q", rec2.Code, rec2.Body.String())
	}
}

func TestDevLoginEndpoint(t *testing.T) {
	srv, _ := testServerWithAuth(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/auth/dev", strings.NewReader(`{"email":"dev@b.c"}`)))
	if rec.Code != 200 {
		t.Fatalf("код %d тело %q", rec.Code, rec.Body.String())
	}
}
