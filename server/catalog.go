package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/kliuchnikovv/dnd/cases"
)

// defaultRulesKind — правила по умолчанию для дел без явного поля "rules" в
// case.json: обратная совместимость с M1a-делами, написанными до появления
// системы правил D&D5e.
const defaultRulesKind = "threshold"

// blurbLimit — сколько рун брифинга показывать в витрине каталога. Больше не
// нужно: это анонс дела, а не сам брифинг.
const blurbLimit = 200

// CaseSummary — сводка о деле для витрины GET /cases. Не полный Config: этого
// достаточно, чтобы клиент показал список дел и дал выбрать одно.
type CaseSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Rules    string `json:"rules"`
	Scenario string `json:"scenario"`
	Blurb    string `json:"blurb"`
}

// CaseCatalog — список загруженных дел, отсортированный по ID. Строится один
// раз при старте сервера (см. LoadCatalog) и раздаётся как есть — дела не
// появляются и не исчезают в рантайме.
type CaseCatalog struct {
	entries []CaseSummary
}

// NewCaseCatalog строит каталог из готовых сводок, сортируя их по ID. Нужен
// тестам и вызывающим, у которых сводки уже собраны без обхода директории.
func NewCaseCatalog(entries []CaseSummary) *CaseCatalog {
	sorted := make([]CaseSummary, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	return &CaseCatalog{entries: sorted}
}

// HandleList — GET /cases: отдаёт каталог JSON-массивом. Пустой каталог отдаёт
// "[]", а не "null" — клиенту не нужно отдельно проверять на null.
func (c *CaseCatalog) HandleList(w http.ResponseWriter, r *http.Request) {
	entries := c.entries
	if entries == nil {
		entries = []CaseSummary{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// rawRules — минимальный вид case.json, достаточный чтобы вытащить поле
// "rules". Полная схема (cases.File) это поле не разбирает: единственную
// систему правил дела подставляет вызывающий сам после cases.Load (см.
// cases/lighthouse/case_test.go). Для витрины каталога этого разбора вручную
// достаточно — трогать схему дела не нужно.
type rawRules struct {
	Rules string `json:"rules"`
	// ID/Blurb читаются только для виньетка-дел (они не идут через cases.Load,
	// у которого свой разбор id/брифинга). Для M1a-дел эти поля игнорируются.
	ID    string `json:"id"`
	Blurb string `json:"blurb"`
}

// LoadCatalog обходит директорию dir в поисках case.json, разбирает каждое
// дело через cases.Load и собирает сводку для витрины /cases. Ошибка в любом
// деле останавливает загрузку целиком: недогруженный каталог на старте сервера
// хуже явного отказа.
func LoadCatalog(dir string) (*CaseCatalog, error) {
	var entries []CaseSummary
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "case.json" {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("catalog: %s: %w", path, err)
		}
		var rr rawRules
		if err := json.Unmarshal(raw, &rr); err != nil {
			return fmt.Errorf("catalog: %s: %w", path, err)
		}
		rules := rr.Rules
		if rules == "" {
			rules = defaultRulesKind
		}

		// Виньетка-дело (ADR-0009) не идёт через M1a-loader: у него другая модель
		// сцены (scene.json), а cases.Load ждёт локации/сущности M1a. Списываем
		// его в каталог по полю-анонсу, не парся как M1a-дело.
		if rules == VignetteRulesKind {
			entries = append(entries, CaseSummary{
				ID:       rr.ID,
				Name:     filepath.Base(filepath.Dir(path)),
				Rules:    rules,
				Scenario: VignetteRulesKind,
				Blurb:    blurb(rr.Blurb),
			})
			return nil
		}

		cfg, err := cases.Load(path)
		if err != nil {
			return fmt.Errorf("catalog: %s: %w", path, err)
		}
		entries = append(entries, CaseSummary{
			ID:       string(cfg.CaseID),
			Name:     filepath.Base(filepath.Dir(path)),
			Rules:    rules,
			Scenario: string(cfg.Scenario.Kind()),
			Blurb:    blurb(cfg.Briefing),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return NewCaseCatalog(entries), nil
}

// blurb — первые ~200 символов брифинга. Первый заход без правки схемы: у
// case.json пока нет отдельного поля-анонса.
func blurb(briefing string) string {
	r := []rune(briefing)
	if len(r) <= blurbLimit {
		return briefing
	}
	return string(r[:blurbLimit])
}
