package server

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kliuchnikovv/dnd/store"
)

var errNotImplemented = errors.New("server: не реализовано (Task 5)")

//go:embed migrations/0001_init.sql
var migration0001 string

// pgStore — долговечный журнал в Postgres. Форма запросов повторяет memStore:
// та же семантика append-only и идемпотентности, только за сетью. Пул pgx
// разделяется всеми сессиями; ходы одной сессии сериализованы rt.mu, поэтому
// гонок за seq внутри сессии нет.
type pgStore struct {
	pool *pgxpool.Pool
}

// NewPgStore подключается к Postgres по DSN и накатывает миграции. Ошибка —
// сеть или схема; сервер без журнала не стартует.
func NewPgStore(ctx context.Context, dsn string) (Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("подключение к Postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("Postgres не отвечает: %w", err)
	}
	s := &pgStore{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// migrate накатывает схему. Все выражения идемпотентны (IF NOT EXISTS), поэтому
// прогон на каждом старте безопасен — отдельная таблица версий в MVP не нужна.
func (s *pgStore) migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, migration0001); err != nil {
		return fmt.Errorf("миграция схемы: %w", err)
	}
	return nil
}

func (s *pgStore) SaveSession(ctx context.Context, rec SessionRecord) error {
	// Upsert: рестарт не должен спотыкаться о уже записанную сессию.
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (chat_id, case_id, seed, snapshot, core_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (chat_id) DO UPDATE SET
			case_id = EXCLUDED.case_id,
			seed = EXCLUDED.seed,
			snapshot = EXCLUDED.snapshot,
			core_version = EXCLUDED.core_version`,
		rec.ChatID, rec.CaseID, rec.Seed, rec.Snapshot, rec.CoreVersion)
	if err != nil {
		return fmt.Errorf("запись сессии: %w", err)
	}
	return nil
}

func (s *pgStore) Sessions(ctx context.Context) ([]SessionRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT chat_id, case_id, seed, snapshot, core_version FROM sessions`)
	if err != nil {
		return nil, fmt.Errorf("чтение сессий: %w", err)
	}
	defer rows.Close()
	var out []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err := rows.Scan(&r.ChatID, &r.CaseID, &r.Seed, &r.Snapshot, &r.CoreVersion); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AppendCommand пишет команду как pending, повторяя семантику store.DB: при
// известном ключе идемпотентности возвращает существующую строку и false.
// Проверка и вставка — в одной транзакции: seq назначается под блокировкой
// строки сессии, чтобы параллельная запись не выдала тот же номер.
func (s *pgStore) AppendCommand(ctx context.Context, e store.CommandLogEntry) (store.CommandLogEntry, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.CommandLogEntry{}, false, err
	}
	defer tx.Rollback(ctx)

	if e.IdempotencyKey != "" {
		existing, ok, err := scanOne(ctx, tx, `
			SELECT session_id, seq, snapshot_id, core_version, intent,
			       dice_seed, dice_turn, idempotency_key, status
			FROM command_log
			WHERE session_id = $1 AND idempotency_key = $2`,
			string(e.SessionID), e.IdempotencyKey)
		if err != nil {
			return store.CommandLogEntry{}, false, err
		}
		if ok {
			return existing, false, tx.Commit(ctx)
		}
	}

	var seq int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(seq), 0) + 1 FROM command_log WHERE session_id = $1`,
		string(e.SessionID)).Scan(&seq); err != nil {
		return store.CommandLogEntry{}, false, err
	}
	e.Seq = seq
	e.Status = store.CommandPending
	if _, err := tx.Exec(ctx, `
		INSERT INTO command_log (session_id, seq, snapshot_id, core_version,
			intent, dice_seed, dice_turn, idempotency_key, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		string(e.SessionID), e.Seq, e.SnapshotID, e.CoreVersion,
		[]byte(e.Intent), e.DiceCtx.Seed, e.DiceCtx.Turn, e.IdempotencyKey, string(e.Status)); err != nil {
		return store.CommandLogEntry{}, false, err
	}
	return e, true, tx.Commit(ctx)
}

func (s *pgStore) MarkApplied(ctx context.Context, session store.SessionID, seq int) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE command_log SET status = $3
		WHERE session_id = $1 AND seq = $2`,
		string(session), seq, string(store.CommandApplied))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("команда не найдена: сессия %q, seq %d", session, seq)
	}
	return nil
}

func (s *pgStore) Commands(ctx context.Context, session store.SessionID) ([]store.CommandLogEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT session_id, seq, snapshot_id, core_version, intent,
		       dice_seed, dice_turn, idempotency_key, status
		FROM command_log WHERE session_id = $1 ORDER BY seq`,
		string(session))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.CommandLogEntry
	for rows.Next() {
		e, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *pgStore) Close(context.Context) error {
	s.pool.Close()
	return nil
}

// scanOne читает одну команду по запросу; ok=false, если строки нет.
func scanOne(ctx context.Context, tx pgx.Tx, sql string, args ...any) (store.CommandLogEntry, bool, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return store.CommandLogEntry{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return store.CommandLogEntry{}, false, rows.Err()
	}
	e, err := scanCommand(rows)
	if err != nil {
		return store.CommandLogEntry{}, false, err
	}
	return e, true, nil
}

// scanCommand разбирает строку command_log в CommandLogEntry.
func scanCommand(rows pgx.Rows) (store.CommandLogEntry, error) {
	var (
		e      store.CommandLogEntry
		sid    string
		intent []byte
		status string
	)
	if err := rows.Scan(&sid, &e.Seq, &e.SnapshotID, &e.CoreVersion, &intent,
		&e.DiceCtx.Seed, &e.DiceCtx.Turn, &e.IdempotencyKey, &status); err != nil {
		return store.CommandLogEntry{}, err
	}
	e.SessionID = store.SessionID(sid)
	e.Intent = append([]byte(nil), intent...)
	e.Status = store.CommandStatus(status)
	return e, nil
}

func (s *pgStore) UpsertUser(context.Context, UserRecord) error {
	// TODO(Task 5): реальная реализация
	return errNotImplemented
}

func (s *pgStore) UserByGoogleSub(context.Context, string) (UserRecord, bool, error) {
	// TODO(Task 5): реальная реализация
	return UserRecord{}, false, errNotImplemented
}

func (s *pgStore) UserByID(context.Context, string) (UserRecord, bool, error) {
	// TODO(Task 5): реальная реализация
	return UserRecord{}, false, errNotImplemented
}

func (s *pgStore) SaveRefresh(context.Context, string, string, time.Time) error {
	// TODO(Task 5): реальная реализация
	return errNotImplemented
}

func (s *pgStore) RefreshOwner(context.Context, string) (string, bool, error) {
	// TODO(Task 5): реальная реализация
	return "", false, errNotImplemented
}

func (s *pgStore) DeleteRefresh(context.Context, string) error {
	// TODO(Task 5): реальная реализация
	return errNotImplemented
}

// Гарантия на этапе компиляции: pgStore реализует Store.
var _ Store = (*pgStore)(nil)
