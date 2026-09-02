package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// TestSessionStartHappyPath — персонаж с ruleset "threshold" стартует дело
// "harbour" (тоже threshold, по умолчанию): 200 и непустой chat_id.
func TestSessionStartHappyPath(t *testing.T) {
	cs := NewCharacterStore()
	cs.Save(&store.Character{ID: "chr_kay_t", Ruleset: "threshold"})
	srv := New(NewManager(casesRoot), WithCharacters(cs))

	rec := postSession(t, srv, map[string]string{"case_id": "harbour", "character_id": "chr_kay_t"})
	if rec.Code != 200 {
		t.Fatalf("code = %d, тело: %s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["chat_id"] == "" {
		t.Fatalf("пустой chat_id: %+v", got)
	}
}

// TestSessionStartRulesetMismatch — персонаж threshold пытается зайти в
// дело lighthouse (ruleset dnd5e): 400 с телом, описывающим несовпадение.
func TestSessionStartRulesetMismatch(t *testing.T) {
	cs := NewCharacterStore()
	cs.Save(&store.Character{ID: "chr_kay_t", Ruleset: "threshold"})
	srv := New(NewManager(casesRoot), WithCharacters(cs))

	rec := postSession(t, srv, map[string]string{"case_id": "lighthouse", "character_id": "chr_kay_t"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, ждали 400, тело: %s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] == "" {
		t.Fatalf("пустое тело ошибки: %+v", got)
	}
}

// TestSessionStartCharacterNotFound — незнакомый character_id: 400, не 500 и
// не тихий фолбэк на легаси-путь.
func TestSessionStartCharacterNotFound(t *testing.T) {
	srv := New(NewManager(casesRoot), WithCharacters(NewCharacterStore()))

	rec := postSession(t, srv, map[string]string{"case_id": "harbour", "character_id": "no-such"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, ждали 400, тело: %s", rec.Code, rec.Body.String())
	}
}

// TestSessionStartWithoutCharacterIDIsLegacy — регрессия моста: запрос без
// character_id продолжает работать (case.json несёт авторского персонажа),
// даже когда character-store подключён.
func TestSessionStartWithoutCharacterIDIsLegacy(t *testing.T) {
	srv := New(NewManager(casesRoot), WithCharacters(NewCharacterStore()))

	rec := postSession(t, srv, map[string]string{"case_id": "harbour"})
	if rec.Code != 200 {
		t.Fatalf("code = %d, тело: %s", rec.Code, rec.Body.String())
	}
}

func postSession(t *testing.T, srv *Server, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/sessions", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}
