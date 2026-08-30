package server

import (
	"context"
	"sync"
	"time"

	"github.com/kliuchnikovv/dnd/store"
)

// UserRecord — профиль пользователя, идентифицированного через Google.
type UserRecord struct {
	ID        string
	GoogleSub string
	Email     string
	Name      string
	Picture   string
}

// SessionRecord — паспорт сессии в журнале: то, из чего сессия восстанавливается
// после рестарта. Состояние игры не хранится — оно выводится реплеем команд из
// (case, seed) детерминированно (ADR-0002).
type SessionRecord struct {
	ChatID      string
	CaseID      string
	Seed        int64
	Snapshot    string
	CoreVersion string
	UserID      string // владелец; пусто у легаси-сессий
	CreatedAt   time.Time
}

// Роли записей ленты.
const (
	RolePlayer = "player" // эхо действия игрока
	RoleGM     = "gm"      // проза Мастера
	RoleNPC    = "npc"     // реплика NPC (прямая речь)
)

// TranscriptEntry — одна запись ленты хода: реплика Мастера/NPC или эхо действия
// игрока. Долговечна (в отличие от буфера прозы в памяти): переживает реконнект
// и рестарт, чтобы клиент показывал всю историю партии, а не только текущий ход.
type TranscriptEntry struct {
	Role    string // RolePlayer | RoleGM | RoleNPC
	Text    string
	Speaker string // имя NPC для role=RoleNPC; пусто иначе
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
	// SessionsByUser — сессии владельца userID, для экрана выбора сессии.
	SessionsByUser(ctx context.Context, userID string) ([]SessionRecord, error)
	// AppendCommand пишет команду как pending. Второй результат — новая ли она:
	// при уже известном ключе идемпотентности возвращается существующая и
	// false, чтобы реплей хвоста не применил ход дважды.
	AppendCommand(ctx context.Context, e store.CommandLogEntry) (store.CommandLogEntry, bool, error)
	// MarkApplied переводит команду pending → applied.
	MarkApplied(ctx context.Context, session store.SessionID, seq int) error
	// Commands — команды сессии в порядке seq: вход реплея.
	Commands(ctx context.Context, session store.SessionID) ([]store.CommandLogEntry, error)
	// AppendTranscript дописывает запись ленты сессии (append-only, порядок
	// сохраняется). В отличие от прозы в памяти — переживает рестарт.
	AppendTranscript(ctx context.Context, session store.SessionID, e TranscriptEntry) error
	// Transcript — лента сессии в порядке добавления: история для клиента.
	Transcript(ctx context.Context, session store.SessionID) ([]TranscriptEntry, error)
	// Close освобождает ресурсы (пул соединений Postgres). Для памяти — no-op.
	Close(ctx context.Context) error

	// UpsertUser сохраняет или обновляет профиль пользователя.
	UpsertUser(ctx context.Context, u UserRecord) error
	// UserByGoogleSub возвращает профиль по Google sub. ok=false, если не найден.
	UserByGoogleSub(ctx context.Context, sub string) (UserRecord, bool, error)
	// UserByID возвращает профиль по ID. ok=false, если не найден.
	UserByID(ctx context.Context, id string) (UserRecord, bool, error)
	// SaveRefresh сохраняет хэш refresh-токена с владельцем и временем истечения.
	SaveRefresh(ctx context.Context, hash, userID string, expiresAt time.Time) error
	// RefreshOwner возвращает владельца хэша (только если он не истёкший).
	// ok=false, если не найден или истёкший.
	RefreshOwner(ctx context.Context, hash string) (userID string, ok bool, err error)
	// DeleteRefresh удаляет хэш refresh-токена (обычно при ротации).
	DeleteRefresh(ctx context.Context, hash string) error
	// ClaimRefresh атомарно забирает refresh-токен: существующий и не
	// истёкший хэш удаляется, возвращая владельца с ok=true; иначе ok=false.
	// Атомарность закрывает окно гонки между чтением владельца и удалением
	// при параллельной ротации одним и тем же токеном.
	ClaimRefresh(ctx context.Context, hash string) (userID string, ok bool, err error)
}

// memRefresh — служебная структура для хранения refresh-токена.
type memRefresh struct {
	userID string
	exp    time.Time
}

// memStore — журнал в памяти. Переживает пересборку Manager (это отдельный
// объект), поэтому годится и как MVP-хранилище без БД, и как эталон реплея в
// тестах: рестарт моделируется новым Manager над тем же memStore. Команды
// делегируются store.DB — ровно той машинерии, что станет Postgres.
type memStore struct {
	mu          sync.Mutex
	db          *store.DB
	sessions    map[string]SessionRecord
	users       map[string]UserRecord      // by ID
	bySub       map[string]string          // google_sub -> ID
	refresh     map[string]memRefresh      // hash -> {userID, exp}
	transcripts map[string][]TranscriptEntry // chatID -> лента в порядке добавления
}

// NewMemStore — пустой журнал в памяти.
func NewMemStore() *memStore {
	return &memStore{
		db:          store.NewDB(),
		sessions:    make(map[string]SessionRecord),
		users:       make(map[string]UserRecord),
		bySub:       make(map[string]string),
		refresh:     make(map[string]memRefresh),
		transcripts: make(map[string][]TranscriptEntry),
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

func (s *memStore) SessionsByUser(_ context.Context, userID string) ([]SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []SessionRecord
	for _, r := range s.sessions {
		if userID != "" && r.UserID == userID {
			out = append(out, r)
		}
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

func (s *memStore) AppendTranscript(_ context.Context, session store.SessionID, e TranscriptEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transcripts[string(session)] = append(s.transcripts[string(session)], e)
	return nil
}

func (s *memStore) Transcript(_ context.Context, session store.SessionID) ([]TranscriptEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.transcripts[string(session)]
	out := make([]TranscriptEntry, len(src))
	copy(out, src)
	return out, nil
}

func (s *memStore) Close(context.Context) error { return nil }

func (s *memStore) UpsertUser(_ context.Context, u UserRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
	if u.GoogleSub != "" {
		s.bySub[u.GoogleSub] = u.ID
	}
	return nil
}

func (s *memStore) UserByGoogleSub(_ context.Context, sub string) (UserRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.bySub[sub]
	if !ok {
		return UserRecord{}, false, nil
	}
	u, ok := s.users[id]
	return u, ok, nil
}

func (s *memStore) UserByID(_ context.Context, id string) (UserRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	return u, ok, nil
}

func (s *memStore) SaveRefresh(_ context.Context, hash, userID string, exp time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh[hash] = memRefresh{userID: userID, exp: exp}
	return nil
}

func (s *memStore) RefreshOwner(_ context.Context, hash string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.refresh[hash]
	if !ok || !r.exp.After(time.Now()) {
		return "", false, nil
	}
	return r.userID, true, nil
}

func (s *memStore) DeleteRefresh(_ context.Context, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.refresh, hash)
	return nil
}

func (s *memStore) ClaimRefresh(_ context.Context, hash string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.refresh[hash]
	if !ok || !r.exp.After(time.Now()) {
		return "", false, nil
	}
	delete(s.refresh, hash)
	return r.userID, true, nil
}
