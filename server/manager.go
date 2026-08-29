// Package server — тонкий транспорт над доменом. Он не считает состояние
// игры: только грузит дело, строит core.Game, разворачивает интент и отдаёт
// turn-view. Пакет живёт НАД чистыми слоями (core/rules/store/dice/cases) и
// вправе знать о сети и моделях — граница держится тем, что чистые слои о нём
// не знают (см. e2e/architecture_test.go).
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

// sessionRuntime — живая игра одной сессии в памяти. В следующих фазах сюда
// добавятся подписчики сокета и буфер прозы; сейчас достаточно самой игры и
// её паспорта, по которому сессия воспроизводится из журнала.
type sessionRuntime struct {
	chatID string
	caseID store.CaseID
	seed   int64
	game   *core.Game
}

// Manager — кэш живых сессий. MVP держит их в памяти; восстановление реплеем
// журнала появится в фазе Postgres. Доступ сериализован мьютексом: сокеты
// приходят из разных горутин.
type Manager struct {
	casesRoot string

	mu       sync.Mutex
	seq      uint64
	sessions map[string]*sessionRuntime
}

// NewManager строит менеджер, ищущий дела в casesRoot: дело "harbour" — это
// файл casesRoot/harbour/case.json.
func NewManager(casesRoot string) *Manager {
	return &Manager{
		casesRoot: casesRoot,
		sessions:  make(map[string]*sessionRuntime),
	}
}

// Create грузит дело, строит игру на данном seed и регистрирует сессию,
// возвращая её chat_id. Ошибка — только про дело: не нашли файл или не
// разобрали. Состояние игры выводится из (дело, seed) детерминированно.
func (m *Manager) Create(caseName string, seed int64) (string, error) {
	if caseName == "" {
		return "", fmt.Errorf("дело не указано")
	}
	path := filepath.Join(m.casesRoot, caseName, "case.json")
	// Сырьё читаем сами: снепшот хешируется от СОДЕРЖИМОГО дела, а не от пути
	// — правка дела обязана делать журнал непереигрываемым (как в cmd/dnd).
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("дело %q: %w", caseName, err)
	}
	cfg, err := cases.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("дело %q: %w", caseName, err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")

	game := core.NewGame(*cfg)
	snap := snapshotID(cfg.CaseID, raw, seed)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	// Два прогона одного дела на одном seed — две разные сессии; порядковый
	// номер процесса их разводит без обращения к часам (тест воспроизводим).
	chatID := fmt.Sprintf("%s-%d", snap, m.seq)
	m.sessions[chatID] = &sessionRuntime{
		chatID: chatID,
		caseID: cfg.CaseID,
		seed:   seed,
		game:   game,
	}
	return chatID, nil
}

// Get возвращает живую сессию по chat_id.
func (m *Manager) Get(chatID string) (*sessionRuntime, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, ok := m.sessions[chatID]
	return rt, ok
}

// snapshotID — паспорт начального состояния: дело плюс seed. Хеш от
// содержимого, а не от пути, чтобы правка дела ломала переигрываемость явно.
func snapshotID(caseID store.CaseID, content []byte, seed int64) string {
	h := sha256.New()
	h.Write(content)
	fmt.Fprintf(h, "|%d", seed)
	return fmt.Sprintf("%s@%s", caseID, hex.EncodeToString(h.Sum(nil))[:8])
}
