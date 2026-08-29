// Package server — тонкий транспорт над доменом. Он не считает состояние
// игры: только грузит дело, строит core.Game, разворачивает интент и отдаёт
// turn-view. Пакет живёт НАД чистыми слоями (core/rules/store/dice/cases) и
// вправе знать о сети и моделях — граница держится тем, что чистые слои о нём
// не знают (см. e2e/architecture_test.go).
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

// sessionRuntime — живая игра одной сессии в памяти вместе с её сокет-
// подписчиками. Генерация хода не привязана к живому сокету: механику применяет
// ядро, результат рассылается всем подписчикам chat_id, поэтому второе
// устройство и реконнект видят один и тот же ход (буфер прозы придёт в фазе 4).
type sessionRuntime struct {
	chatID   string
	caseID   store.CaseID
	seed     int64
	snapshot string
	store    Store

	// narrator — Мастер для стрима прозы. nil означает механику без прозы
	// (сервер без сконфигурированного LLM): ходы применяются, session_state
	// уходит, прозы просто нет.
	narrator *master.Master

	mu   sync.Mutex
	game *core.Game
	// turn — номер применённого хода. Входит в DiceCtx и ключ идемпотентности
	// журнала; растёт на каждый состоявшийся ход.
	turn int
	// Стрим прозы текущего хода: буфер дельт под досстрим при возобновлении,
	// признак «идёт генерация» и отмена (op:stop и суперсессия новым ходом).
	narration  []string
	narrating  bool
	narrateGen int
	genCancel  context.CancelFunc
	// spokenTo — последний адресат: разговор продолжается с тем же NPC, пока
	// игрок не обратится к другому. Определяет набор аффордансов хода.
	spokenTo store.EntityID
	// lastAppliedID — наибольший applied Frame.ID: клиент шлёт монотонный
	// счётчик, переживающий реконнект, поэтому повтор (id <= lastApplied) —
	// это дубль доставки, а не новый ход. Идемпотентность хода.
	lastAppliedID int
	// subs — открытые сокеты этой сессии. Рассылка session_state идёт всем.
	subs map[*subscriber]struct{}
	// outSeq — счётчик id серверных кадров (session_state, error): id входящих
	// принадлежит клиенту, исходящие нумеруются сервером.
	outSeq int
}

// subscriber — один подключённый сокет. out буферизован: медленный клиент не
// держит горутину хода; переполнение означает отставшего читателя (фаза 5
// решит его реконнектом с досстримом).
type subscriber struct {
	out chan Frame
}

// Manager — кэш живых сессий над долговечным журналом. Сессии, которых нет в
// кэше (после рестарта), восстанавливаются реплеем журнала при первом
// обращении. Доступ сериализован мьютексом: сокеты приходят из разных горутин.
type Manager struct {
	casesRoot string
	store     Store
	// narrator — общий Мастер для стрима прозы, один на все сессии (шлюз и
	// ledger потокобезопасны). nil означает сервер без прозы.
	narrator *master.Master

	mu       sync.Mutex
	sessions map[string]*sessionRuntime
}

// WithNarrator включает стрим прозы: сессии получат Мастера. Без него сервер
// отдаёт только механику (session_state), как в фазах до LLM.
func (m *Manager) WithNarrator(ms *master.Master) *Manager {
	m.narrator = ms
	return m
}

// NewManager строит менеджер с журналом в памяти — MVP без БД и основа тестов.
// Дела ищутся в casesRoot: дело "harbour" — это casesRoot/harbour/case.json.
func NewManager(casesRoot string) *Manager {
	return NewManagerWithStore(casesRoot, NewMemStore())
}

// NewManagerWithStore — менеджер над заданным журналом (Postgres в проде).
func NewManagerWithStore(casesRoot string, st Store) *Manager {
	return &Manager{
		casesRoot: casesRoot,
		store:     st,
		sessions:  make(map[string]*sessionRuntime),
	}
}

// Create грузит дело, строит игру на данном seed, записывает паспорт сессии в
// журнал и регистрирует её, возвращая chat_id. Ошибка — про дело (не нашли/не
// разобрали) или про журнал.
func (m *Manager) Create(caseName string, seed int64) (string, error) {
	game, caseID, snapshot, err := m.buildGame(caseName, seed)
	if err != nil {
		return "", err
	}
	chatID := fmt.Sprintf("%s-%s", snapshot, randToken())

	if err := m.store.SaveSession(context.Background(), SessionRecord{
		ChatID:      chatID,
		CaseID:      caseName,
		Seed:        seed,
		Snapshot:    snapshot,
		CoreVersion: core.Version,
	}); err != nil {
		return "", fmt.Errorf("журнал: %w", err)
	}

	rt := &sessionRuntime{
		chatID:   chatID,
		caseID:   caseID,
		seed:     seed,
		snapshot: snapshot,
		store:    m.store,
		narrator: m.narrator,
		game:     game,
		subs:     make(map[*subscriber]struct{}),
	}
	m.mu.Lock()
	m.sessions[chatID] = rt
	m.mu.Unlock()
	return chatID, nil
}

// Get возвращает живую сессию по chat_id, восстанавливая её из журнала, если в
// кэше нет (первое обращение после рестарта). Второй результат — нашлась ли
// сессия вообще.
func (m *Manager) Get(chatID string) (*sessionRuntime, bool) {
	m.mu.Lock()
	rt, ok := m.sessions[chatID]
	m.mu.Unlock()
	if ok {
		return rt, true
	}
	return m.reconstruct(chatID)
}

// reconstruct поднимает сессию из журнала: паспорт → свежая игра из (case,
// seed) → реплей команд тем же путём, что живой ход. Незавершённый хвост
// (pending после падения между «записал» и «применил») доигрывается и
// помечается applied. Двойного применения нет: реплей журнал не дописывает.
func (m *Manager) reconstruct(chatID string) (*sessionRuntime, bool) {
	ctx := context.Background()
	recs, err := m.store.Sessions(ctx)
	if err != nil {
		return nil, false
	}
	var rec SessionRecord
	found := false
	for _, r := range recs {
		if r.ChatID == chatID {
			rec, found = r, true
			break
		}
	}
	if !found {
		return nil, false
	}

	game, caseID, snapshot, err := m.buildGame(rec.CaseID, rec.Seed)
	if err != nil {
		return nil, false
	}
	rt := &sessionRuntime{
		chatID:   chatID,
		caseID:   caseID,
		seed:     rec.Seed,
		snapshot: snapshot,
		store:    m.store,
		narrator: m.narrator,
		game:     game,
		subs:     make(map[*subscriber]struct{}),
	}

	cmds, err := m.store.Commands(ctx, store.SessionID(chatID))
	if err != nil {
		return nil, false
	}
	for _, e := range cmds {
		if e.CoreVersion != core.Version {
			// Правка ядра меняет исход прошлых бросков: реплей обязан сказать
			// это, а не выдать другую сессию за ту же. Молча продолжить нельзя.
			return nil, false
		}
		var intent core.Intent
		if err := json.Unmarshal(e.Intent, &intent); err != nil {
			return nil, false
		}
		rt.advance(intent)
		rt.turn = e.DiceCtx.Turn
		if e.Status == store.CommandPending {
			_ = m.store.MarkApplied(ctx, e.SessionID, e.Seq)
		}
	}

	m.mu.Lock()
	// Гонка: пока восстанавливали, другой мог уже поднять эту сессию. Первый
	// победитель остаётся, чтобы не расщепить состояние на два объекта.
	if existing, ok := m.sessions[chatID]; ok {
		m.mu.Unlock()
		return existing, true
	}
	m.sessions[chatID] = rt
	m.mu.Unlock()
	return rt, true
}

// WarmCache поднимает все сессии журнала в кэш. cmd/server зовёт её на старте:
// клиент, цепляющийся сразу после рестарта, не ждёт ленивого восстановления.
func (m *Manager) WarmCache(ctx context.Context) error {
	recs, err := m.store.Sessions(ctx)
	if err != nil {
		return err
	}
	for _, r := range recs {
		m.Get(r.ChatID)
	}
	return nil
}

// buildGame собирает игру из (дело, seed) — общий путь для Create и реплея.
// Возвращает игру, её caseID и снепшот начального состояния.
func (m *Manager) buildGame(caseName string, seed int64) (*core.Game, store.CaseID, string, error) {
	if caseName == "" {
		return nil, "", "", fmt.Errorf("дело не указано")
	}
	path := filepath.Join(m.casesRoot, caseName, "case.json")
	// Сырьё читаем сами: снепшот хешируется от СОДЕРЖИМОГО дела, а не от пути
	// — правка дела обязана делать журнал непереигрываемым (как в cmd/dnd).
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("дело %q: %w", caseName, err)
	}
	cfg, err := cases.Parse(raw)
	if err != nil {
		return nil, "", "", fmt.Errorf("дело %q: %w", caseName, err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	return core.NewGame(*cfg), cfg.CaseID, snapshotID(cfg.CaseID, raw, seed), nil
}

// randToken — короткий случайный суффикс chat_id. Разводит две сессии одного
// дела на одном seed и переживает рестарт: счётчик процесса дал бы после
// перезапуска тот же id и столкновение в журнале.
func randToken() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// snapshotID — паспорт начального состояния: дело плюс seed. Хеш от
// содержимого, а не от пути, чтобы правка дела ломала переигрываемость явно.
func snapshotID(caseID store.CaseID, content []byte, seed int64) string {
	h := sha256.New()
	h.Write(content)
	fmt.Fprintf(h, "|%d", seed)
	return fmt.Sprintf("%s@%s", caseID, hex.EncodeToString(h.Sum(nil))[:8])
}
