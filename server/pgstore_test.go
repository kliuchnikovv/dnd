package server

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// pgStoreForTest подключается к Postgres из DATABASE_URL. Без него интеграция
// пропускается — go test ./... остаётся зелёным везде, а против реальной БД
// (локально/CI с Postgres) контракт pgStore проверяется тем же кодом.
func pgStoreForTest(t *testing.T) Store {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL не задан — пропускаем интеграцию Postgres")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := NewPgStore(ctx, dsn)
	if err != nil {
		t.Fatalf("подключение к Postgres: %v", err)
	}
	return st
}

// pgStore реализует контракт Store: запись команды, дубль по ключу
// идемпотентности, отметка applied, чтение по порядку.
func TestPgStoreContract(t *testing.T) {
	st := pgStoreForTest(t)
	defer st.Close(context.Background())
	ctx := context.Background()

	chatID := "test@" + randToken()
	if err := st.SaveSession(ctx, SessionRecord{
		ChatID: chatID, CaseID: "harbour", Seed: 1,
		Snapshot: "snap", CoreVersion: core.Version,
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	dctx := store.DiceCtx{Seed: 1, Turn: 1}
	payload := json.RawMessage(`{"verb":"examine"}`)
	entry := store.CommandLogEntry{
		SessionID: store.SessionID(chatID), SnapshotID: "snap",
		CoreVersion: core.Version, Intent: payload, DiceCtx: dctx,
		IdempotencyKey: idempotencyKey(store.SessionID(chatID), dctx, payload),
	}
	saved, isNew, err := st.AppendCommand(ctx, entry)
	if err != nil || !isNew {
		t.Fatalf("первая запись: isNew=%v err=%v", isNew, err)
	}
	if saved.Seq != 1 {
		t.Fatalf("seq первой команды %d, ждали 1", saved.Seq)
	}

	// Тот же ключ идемпотентности — существующая строка и false.
	again, isNew, err := st.AppendCommand(ctx, entry)
	if err != nil {
		t.Fatalf("повтор записи: %v", err)
	}
	if isNew {
		t.Fatalf("дубль по ключу идемпотентности записался как новый")
	}
	if again.Seq != saved.Seq {
		t.Fatalf("дубль получил другой seq: %d vs %d", again.Seq, saved.Seq)
	}

	if err := st.MarkApplied(ctx, store.SessionID(chatID), saved.Seq); err != nil {
		t.Fatalf("MarkApplied: %v", err)
	}
	cmds, err := st.Commands(ctx, store.SessionID(chatID))
	if err != nil || len(cmds) != 1 {
		t.Fatalf("Commands: %d строк, err=%v", len(cmds), err)
	}
	if cmds[0].Status != store.CommandApplied {
		t.Fatalf("статус %s, ждали applied", cmds[0].Status)
	}
}

// Сессия переживает рестарт и на Postgres: новый Manager над тем же пулом
// восстанавливает то же состояние, включая владельца (user_id — FK на users,
// поэтому владелец сперва заводится через UpsertUser).
func TestPgStoreSurvivesRestart(t *testing.T) {
	st := pgStoreForTest(t)
	defer st.Close(context.Background())
	ctx := context.Background()

	ownerID := "owner-" + randToken()
	if err := st.UpsertUser(ctx, UserRecord{ID: ownerID, Email: "owner@x.test"}); err != nil {
		t.Fatalf("завести владельца: %v", err)
	}

	m1 := NewManagerWithStore(casesRoot, st)
	id, err := m1.Create("harbour", 1, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	rt1, _ := m1.Get(id)
	playTurns(t, rt1, 3)
	wantView := viewJSON(t, rt1)
	wantNode := rt1.game.Node

	m2 := NewManagerWithStore(casesRoot, st)
	rt2, ok := m2.Get(id)
	if !ok {
		t.Fatal("сессия не восстановилась из Postgres")
	}
	if rt2.game.Node != wantNode {
		t.Fatalf("узел после рестарта %s, был %s", rt2.game.Node, wantNode)
	}
	if got := viewJSON(t, rt2); got != wantView {
		t.Fatalf("состояние разошлось после рестарта на Postgres")
	}
	// Владение обязано пережить рестарт: иначе легитимный владелец получает
	// 403 на реконнекте, потому что rt.userID потерялся при реплее из БД.
	if rt2.userID != rt1.userID {
		t.Fatalf("userID после рестарта %q, был %q", rt2.userID, rt1.userID)
	}
	if rt2.userID != ownerID {
		t.Fatalf("userID после рестарта %q, ждали %q", rt2.userID, ownerID)
	}
}

// pgStore реализует контракт users/refresh: upsert идемпотентен по
// google_sub, refresh-токен виден до удаления и невидим после.
func TestPgStoreUsersAndRefresh(t *testing.T) {
	st := pgStoreForTest(t)
	defer st.Close(context.Background())
	ctx := context.Background()

	id := "u-" + randToken()
	sub := "g-" + randToken()
	if err := st.UpsertUser(ctx, UserRecord{ID: id, GoogleSub: sub, Email: "a@b.c", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := st.UserByGoogleSub(ctx, sub)
	if !ok || got.ID != id {
		t.Fatalf("по sub: %+v ok=%v", got, ok)
	}
	// upsert идемпотентен по google_sub
	if err := st.UpsertUser(ctx, UserRecord{ID: id, GoogleSub: sub, Email: "a2@b.c", Name: "Ann2"}); err != nil {
		t.Fatalf("повторный upsert: %v", err)
	}

	h := "h-" + randToken()
	if err := st.SaveRefresh(ctx, h, id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	owner, ok, _ := st.RefreshOwner(ctx, h)
	if !ok || owner != id {
		t.Fatalf("refresh owner: %q ok=%v", owner, ok)
	}
	_ = st.DeleteRefresh(ctx, h)
	if _, ok, _ := st.RefreshOwner(ctx, h); ok {
		t.Fatal("удалённый refresh жив")
	}

	// ClaimRefresh одноразов: первый вызов забирает владельца и гасит хэш
	// атомарно (DELETE ... RETURNING в одном round-trip), второй уже не
	// находит строку. Это и закрывает окно гонки параллельного refresh.
	h2 := "h2-" + randToken()
	if err := st.SaveRefresh(ctx, h2, id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	owner2, ok, err := st.ClaimRefresh(ctx, h2)
	if err != nil || !ok || owner2 != id {
		t.Fatalf("первый ClaimRefresh: owner=%q ok=%v err=%v", owner2, ok, err)
	}
	owner2, ok, err = st.ClaimRefresh(ctx, h2)
	if err != nil || ok || owner2 != "" {
		t.Fatalf("второй ClaimRefresh должен провалиться: owner=%q ok=%v err=%v", owner2, ok, err)
	}
}

// SessionsByUser отдаёт только сессии владельца, с заполненными UserID и
// CreatedAt.
func TestPgStoreSessionsByUser(t *testing.T) {
	st := pgStoreForTest(t)
	defer st.Close(context.Background())
	ctx := context.Background()
	uid := "u-" + randToken()
	_ = st.UpsertUser(ctx, UserRecord{ID: uid, Email: "o@x"})
	_ = st.SaveSession(ctx, SessionRecord{ChatID: "c-" + randToken(), CaseID: "harbour", Seed: 1, UserID: uid, CoreVersion: "core-1", Snapshot: "s"})
	got, err := st.SessionsByUser(ctx, uid)
	if err != nil || len(got) != 1 {
		t.Fatalf("SessionsByUser: %d rows err=%v", len(got), err)
	}
	if got[0].UserID != uid || got[0].CreatedAt.IsZero() {
		t.Fatalf("owner/created_at не заполнены: %+v", got[0])
	}
}
