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

//go:embed migrations/0001_init.sql
var migration0001 string

//go:embed migrations/0002_auth.sql
var migration0002 string

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
	if _, err := s.pool.Exec(ctx, migration0002); err != nil {
		return fmt.Errorf("миграция 0002: %w", err)
	}
	return nil
}

func (s *pgStore) SaveSession(ctx context.Context, rec SessionRecord) error {
	// Upsert: рестарт не должен спотыкаться о уже записанную сессию.
	// user_id — FK на users(id): пустой UserID (легаси-сессии, сервер без
	// auth) обязан лечь как NULL, а не как "" — иначе вставка упадёт по
	// внешнему ключу на пустую строку, которой нет и не будет в users.
	var userID *string
	if rec.UserID != "" {
		userID = &rec.UserID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (chat_id, case_id, seed, snapshot, core_version, user_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (chat_id) DO UPDATE SET
			case_id = EXCLUDED.case_id,
			seed = EXCLUDED.seed,
			snapshot = EXCLUDED.snapshot,
			core_version = EXCLUDED.core_version,
			user_id = EXCLUDED.user_id`,
		rec.ChatID, rec.CaseID, rec.Seed, rec.Snapshot, rec.CoreVersion, userID)
	if err != nil {
		return fmt.Errorf("запись сессии: %w", err)
	}
	return nil
}

func (s *pgStore) Sessions(ctx context.Context) ([]SessionRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT chat_id, case_id, seed, snapshot, core_version, user_id FROM sessions`)
	if err != nil {
		return nil, fmt.Errorf("чтение сессий: %w", err)
	}
	defer rows.Close()
	var out []SessionRecord
	for rows.Next() {
		var r SessionRecord
		var userID *string
		if err := rows.Scan(&r.ChatID, &r.CaseID, &r.Seed, &r.Snapshot, &r.CoreVersion, &userID); err != nil {
			return nil, err
		}
		r.UserID = deref(userID)
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

func (s *pgStore) UpsertUser(ctx context.Context, u UserRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, google_sub, email, name, picture)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (google_sub) DO UPDATE SET
			email=EXCLUDED.email, name=EXCLUDED.name, picture=EXCLUDED.picture`,
		u.ID, u.GoogleSub, u.Email, u.Name, u.Picture)
	return err
}

func (s *pgStore) UserByGoogleSub(ctx context.Context, sub string) (UserRecord, bool, error) {
	return s.scanUser(ctx, `SELECT id,google_sub,email,name,picture FROM users WHERE google_sub=$1`, sub)
}

func (s *pgStore) UserByID(ctx context.Context, id string) (UserRecord, bool, error) {
	return s.scanUser(ctx, `SELECT id,google_sub,email,name,picture FROM users WHERE id=$1`, id)
}

func (s *pgStore) scanUser(ctx context.Context, sql string, arg string) (UserRecord, bool, error) {
	var u UserRecord
	var sub, name, pic *string
	err := s.pool.QueryRow(ctx, sql, arg).Scan(&u.ID, &sub, &u.Email, &name, &pic)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	if err != nil {
		return UserRecord{}, false, err
	}
	u.GoogleSub, u.Name, u.Picture = deref(sub), deref(name), deref(pic)
	return u, true, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *pgStore) SaveRefresh(ctx context.Context, hash, userID string, exp time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at) VALUES ($1,$2,$3)`,
		hash, userID, exp)
	return err
}

func (s *pgStore) RefreshOwner(ctx context.Context, hash string) (string, bool, error) {
	var uid string
	err := s.pool.QueryRow(ctx, `
		SELECT user_id FROM refresh_tokens WHERE token_hash=$1 AND expires_at > now()`, hash).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return uid, err == nil, err
}

func (s *pgStore) DeleteRefresh(ctx context.Context, hash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash=$1`, hash)
	return err
}

// Гарантия на этапе компиляции: pgStore реализует Store.
var _ Store = (*pgStore)(nil)
