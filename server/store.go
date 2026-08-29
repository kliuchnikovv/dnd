package server

import (
	"context"
	"sync"

	"github.com/kliuchnikovv/dnd/store"
)

// SessionRecord — паспорт сессии в журнале: то, из чего сессия восстанавливается
// после рестарта. Состояние игры не хранится — оно выводится реплеем команд из
// (case, seed) детерминированно (ADR-0002).
type SessionRecord struct {
	ChatID      string
	CaseID      string
	Seed        int64
	Snapshot    string
	CoreVersion string
}

// Store — долговечный журнал сервера. Форма повторяет store.DB (тот уже «в форме
// будущей схемы Postgres»): команды append-only с ключом (session_id, seq),
// интент — сырой JSON. Реализаций две: in-memory (тесты, запуск без БД) и
// Postgres. Логика реплея живёт НАД интерфейсом и одинакова для обеих.
type Store interface {
	// SaveSession регистрирует сессию. Повтор того же chat_id — не ошибка
	// (upsert): рестарт не должен спотыкаться о уже записанную сессию.
	SaveSession(ctx context.Context, rec SessionRecord) error
	// Sessions — все известные сессии: из них сервер прогревает кэш на старте.
	Sessions(ctx context.Context) ([]SessionRecord, error)
	// AppendCommand пишет команду как pending. Второй результат — новая ли она:
	// при уже известном ключе идемпотентности возвращается существующая и
	// false, чтобы реплей хвоста не применил ход дважды.
	AppendCommand(ctx context.Context, e store.CommandLogEntry) (store.CommandLogEntry, bool, error)
	// MarkApplied переводит команду pending → applied.
	MarkApplied(ctx context.Context, session store.SessionID, seq int) error
	// Commands — команды сессии в порядке seq: вход реплея.
	Commands(ctx context.Context, session store.SessionID) ([]store.CommandLogEntry, error)
	// Close освобождает ресурсы (пул соединений Postgres). Для памяти — no-op.
	Close(ctx context.Context) error
}

// memStore — журнал в памяти. Переживает пересборку Manager (это отдельный
// объект), поэтому годится и как MVP-хранилище без БД, и как эталон реплея в
// тестах: рестарт моделируется новым Manager над тем же memStore. Команды
// делегируются store.DB — ровно той машинерии, что станет Postgres.
type memStore struct {
	mu       sync.Mutex
	db       *store.DB
	sessions map[string]SessionRecord
}

// NewMemStore — пустой журнал в памяти.
func NewMemStore() *memStore {
	return &memStore{
		db:       store.NewDB(),
		sessions: make(map[string]SessionRecord),
	}
}

func (s *memStore) SaveSession(_ context.Context, rec SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[rec.ChatID] = rec
	return nil
}

func (s *memStore) Sessions(_ context.Context) ([]SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SessionRecord, 0, len(s.sessions))
	for _, r := range s.sessions {
		out = append(out, r)
	}
	return out, nil
}

func (s *memStore) AppendCommand(_ context.Context, e store.CommandLogEntry) (store.CommandLogEntry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, isNew := s.db.AppendCommand(e)
	return saved, isNew, nil
}

func (s *memStore) MarkApplied(_ context.Context, session store.SessionID, seq int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.MarkApplied(session, seq)
}

func (s *memStore) Commands(_ context.Context, session store.SessionID) ([]store.CommandLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Commands(session), nil
}

func (s *memStore) Close(context.Context) error { return nil }
