package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// TestSessionStartRoutesCharacterSheet — регрессия ревью Task 6: сервер
// сверял character.Ruleset, но сам персонаж в игру не доезжал — buildGame
// заводил дефолтную заглушку (Grit:3, пустой Sheet) вместо настоящего листа.
// Дело "lighthouse" (dnd5e, actor "chr_kay") стартует с реальным персонажем
// произвольного ID из CharacterStore — лист, ruleset и grit обязаны попасть
// в db.Characters[cfg.Actor] той самой игры, а не остаться заглушкой.
//
// Ключ в db.Characters — это cfg.Actor ("chr_kay" в lighthouse), а не
// собственный ID персонажа в CharacterStore: cfg.Actor — тот ключ, под
// которым дело ждёт актёра во всех остальных таблицах (в частности,
// db.Entities для сценария adventure — см. buildGame). Подмена g.Actor на
// произвольный character_id без парной Entity увела бы actor-сущность в
// нулевой HP и сценарий adventure — в мгновенное поражение.
func TestSessionStartRoutesCharacterSheet(t *testing.T) {
	sheet := json.RawMessage(`{"str":16,"dex":14,"prof":3,"max_hp":18,"ac":15,"weapons":[{"name":"меч"}]}`)
	cs := NewCharacterStore()
	cs.Save(&store.Character{ID: "chr_alice_dnd", Ruleset: "dnd5e", Sheet: sheet, Grit: 5})
	srv := New(NewManager(casesRoot), WithCharacters(cs))

	rec := postSession(t, srv, map[string]string{"case_id": "lighthouse", "character_id": "chr_alice_dnd"})
	if rec.Code != 200 {
		t.Fatalf("code = %d, тело: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		ChatID string `json:"chat_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	rt, ok := srv.mgr.Get(got.ChatID)
	if !ok {
		t.Fatalf("сессия %q не найдена в менеджере", got.ChatID)
	}
	ch, ok := rt.game.DB.Characters[rt.game.Actor]
	if !ok {
		t.Fatalf("персонаж-актёр %q не найден в игре", rt.game.Actor)
	}
	if ch.Ruleset != "dnd5e" {
		t.Errorf("ruleset в игре = %q, ожидался dnd5e — сработала заглушка вместо реального персонажа", ch.Ruleset)
	}
	if ch.Grit != 5 {
		t.Errorf("grit в игре = %d, ожидалось 5 (значение реального персонажа, не дефолтная заглушка 3)", ch.Grit)
	}
	if string(ch.Sheet) != string(sheet) {
		t.Errorf("sheet в игре = %s, ожидался %s — реальный лист персонажа не доехал", ch.Sheet, sheet)
	}
}

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

// TestSessionStartWithoutCharacterIDIsRejected — легаси-мост снесён (Task 6):
// character в case.json больше не живёт, и без character_id серверу неоткуда
// взять лист персонажа. 400, а не тихий фолбэк.
func TestSessionStartWithoutCharacterIDIsRejected(t *testing.T) {
	srv := New(NewManager(casesRoot), WithCharacters(NewCharacterStore()))

	rec := postSession(t, srv, map[string]string{"case_id": "harbour"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, ждали 400, тело: %s", rec.Code, rec.Body.String())
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
