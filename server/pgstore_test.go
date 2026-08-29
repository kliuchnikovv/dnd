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
// восстанавливает то же состояние.
func TestPgStoreSurvivesRestart(t *testing.T) {
	st := pgStoreForTest(t)
	defer st.Close(context.Background())

	m1 := NewManagerWithStore(casesRoot, st)
	id, err := m1.Create("harbour", 1)
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
}
