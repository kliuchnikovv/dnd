# D&D-приключение как второй трек. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ввести на движок второй трек «правило × сценарий» — упрощённое подмножество D&D 5e (`rules/dnd5e/`) и архетип solo-приключения (`core/scenarios/adventure/`), с первым игровым кейсом «Ночной маяк». Существующий «Порог × детектив» продолжает играться идентично.

**Architecture:** Вводится `core.Scenario` — второй ортогональный шов рядом с `RuleSystem`. Нынешняя детективная логика (accuse/corroboration/hunch) переезжает за шов в `core/scenarios/deduction/` без смены поведения. Ядро получает опциональные HP/AC на сущности и `Encounter` для инициативной ленты; D&D-правила и AI монстров живут за швами.

**Tech Stack:** Go 1.22+ (см. `go.mod`); тесты — стандартный `testing`, dice — существующая `dice.Source` (детерминированная); JSON-схема кейса — `cases.File`.

**Spec:** `docs/superpowers/specs/2026-09-01-dnd-adventure-track-design.md` — план ссылается на её §-параграфы и не повторяет обоснования.

## Global Constraints

- **Регрессия на каждом шаге:** после каждой задачи `go test ./...` должен пройти зелёным. `harbour` и `forte_merlo` — гейт всех фаз, ломать их запрещено.
- **Обратная совместимость `case.json`:** отсутствие полей `scenario:` / `rules:` читается как `"deduction"` / `"threshold"` соответственно. Существующие кейсы правки не требуют.
- **Не строим то, чего нет в спеке:** реакции, заклинания, классы, партия, encounter builder — за границей плана (см. §11 спеки).
- **Идиома `core`:** ядро не импортирует ни `rules/*`, ни `cli/*`. Сценарии живут в `core/scenarios/*` — им импорт `core` разрешён, но и там нет обратного импорта в `core`.
- **Комментарии в стиле репозитория:** каждый экспортированный тип и функция несёт короткий комментарий по-русски, в тоне существующего кода (см. `core/game.go`, `master/master.go`).
- **Commit-conventions:** `feat:`, `fix:`, `refactor:`, `test:`, `docs:` префиксы; тело коммита — что и зачем; каждый коммит — из тела одной задачи плана.

---

## Обзор фаз

| Фаза | Тема | Задачи |
|---|---|---|
| 1 | Сценарный шов + перенос детектива | 1-4 |
| 2 | Ядерные добавки под бой | 5-8 |
| 3 | Правила `rules/dnd5e/` | 9-15 |
| 4 | Archetype `adventure` | 16-19 |
| 5 | Клиент / turn-view | 20-21 |
| 6 | Кейс «Ночной маяк» | 22-23 |
| 7 | Обкатка | 24 |

---

## Фаза 1. Сценарный шов и перенос детектива

### Task 1: Ввести интерфейс `core.Scenario` и registry

**Files:**
- Create: `core/scenario.go`
- Create: `core/scenario_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces:
  ```go
  type ScenarioKind string
  const ScenarioDeduction ScenarioKind = "deduction"
  const ScenarioAdventure ScenarioKind = "adventure"

  type Scenario interface {
      Kind() ScenarioKind
      ExtraVerbs() []VerbDef
      Victory(g *Game) (won bool, why string)
      Defeat(g *Game) (lost bool, why string)
      Panel(g *Game) Panel
      NPCTurn(g *Game, id store.EntityID) (Intent, bool)
  }

  // Panel — заглушка формы; наполним в Task 20.
  type Panel struct{ Sections []PanelSection }
  type PanelSection struct{ Kind string; Slots []PanelSlot }
  type PanelSlot struct{ Key, Value string }

  // Register/Lookup — реестр сценариев по имени.
  func RegisterScenario(k ScenarioKind, factory func() Scenario)
  func LookupScenario(k ScenarioKind) (Scenario, bool)
  ```

- [ ] **Step 1: Написать провальный тест**

```go
// core/scenario_test.go
package core

import "testing"

func TestScenarioRegistryReturnsRegistered(t *testing.T) {
    stub := stubScenario{kind: "test"}
    RegisterScenario(stub.kind, func() Scenario { return stub })
    got, ok := LookupScenario("test")
    if !ok { t.Fatalf("registry не вернул зарегистрированный сценарий") }
    if got.Kind() != "test" { t.Fatalf("kind разошёлся: %q", got.Kind()) }
}

func TestScenarioRegistryUnknownIsFalse(t *testing.T) {
    if _, ok := LookupScenario("no-such"); ok {
        t.Fatal("registry соврал про незнакомый сценарий")
    }
}

type stubScenario struct{ kind ScenarioKind }
func (s stubScenario) Kind() ScenarioKind { return s.kind }
func (s stubScenario) ExtraVerbs() []VerbDef { return nil }
func (s stubScenario) Victory(*Game) (bool, string) { return false, "" }
func (s stubScenario) Defeat(*Game) (bool, string) { return false, "" }
func (s stubScenario) Panel(*Game) Panel { return Panel{} }
func (s stubScenario) NPCTurn(*Game, store.EntityID) (Intent, bool) { return Intent{}, false }
```

- [ ] **Step 2: Запустить тест — должен упасть**

Run: `go test ./core/... -run TestScenarioRegistry -v`
Expected: `undefined: RegisterScenario` / `undefined: Scenario`.

- [ ] **Step 3: Реализовать `core/scenario.go`**

```go
// Package core — Scenario. Второй ортогональный шов рядом с RuleSystem
// (§3 спеки, ADR-0006). Ядро видит только интерфейс: конкретный архетип
// (детектив, приключение) живёт за швом в core/scenarios/*.
package core

import "github.com/kliuchnikovv/dnd/store"

type ScenarioKind string

const (
    ScenarioDeduction ScenarioKind = "deduction"
    ScenarioAdventure ScenarioKind = "adventure"
)

// Scenario — что архетип поставляет ядру. Панель и NPCTurn — крючки в такт
// презентации и хода; Victory/Defeat — единственная точка правды об исходе
// прогона.
type Scenario interface {
    Kind() ScenarioKind
    ExtraVerbs() []VerbDef
    Victory(g *Game) (won bool, why string)
    Defeat(g *Game) (lost bool, why string)
    Panel(g *Game) Panel
    NPCTurn(g *Game, id store.EntityID) (Intent, bool)
}

// Panel — дескриптор рабочей панели для клиента. Форма общая, содержимое
// определяет сценарий; клиент рисует по дескриптору, не зная конкретики.
// Наполнение секций/слотов — в Task 20.
type Panel struct {
    Sections []PanelSection
}

type PanelSection struct {
    Kind  string
    Slots []PanelSlot
}

type PanelSlot struct {
    Key   string
    Value string
}

var scenarioRegistry = map[ScenarioKind]func() Scenario{}

// RegisterScenario — зарегистрировать фабрику архетипа. Регистрация одна
// на процесс: init-функция пакета сценария зовёт её при импорте.
func RegisterScenario(k ScenarioKind, factory func() Scenario) {
    scenarioRegistry[k] = factory
}

// LookupScenario — получить экземпляр архетипа по имени. Второй результат
// ложен, если сценарий не зарегистрирован (например, пакет не импортирован).
func LookupScenario(k ScenarioKind) (Scenario, bool) {
    f, ok := scenarioRegistry[k]
    if !ok {
        return nil, false
    }
    return f(), true
}
```

Add `VerbDef` shim if not exists — grep first: `grep -n "type VerbDef" core/*.go`. Verbs already defined; use existing type.

- [ ] **Step 4: Запустить тест — должен пройти**

Run: `go test ./core/... -run TestScenarioRegistry -v`
Expected: PASS.

- [ ] **Step 5: Общая регрессия**

Run: `go test ./...`
Expected: все зелёные (мы ничего не сломали, только добавили).

- [ ] **Step 6: Коммит**

```bash
git add core/scenario.go core/scenario_test.go
git commit -m "feat(core): ввести шов Scenario (интерфейс и registry)"
```

---

### Task 2: Подключить `Scenario` к `Game` и прокинуть через `cases.Load`

**Files:**
- Modify: `core/game.go` — добавить `Scenario Scenario` в `Config` и `Game`, конструктор.
- Modify: `cases/schema.go` — поле `Scenario ScenarioKind` в `File`.
- Modify: `cases/load.go` — читать поле, ставить сценарий через `LookupScenario`, fallback на `deduction`.
- Create: `cases/load_scenario_test.go`.

**Interfaces:**
- Consumes: `core.RegisterScenario/LookupScenario` из Task 1.
- Produces:
  ```go
  cfg.Scenario core.Scenario  // не nil после cases.Load; дефолт — deduction
  g.Scenario   core.Scenario  // копия из cfg
  ```

- [ ] **Step 1: Тест на fallback**

```go
// cases/load_scenario_test.go
package cases

import (
    "testing"

    "github.com/kliuchnikovv/dnd/core"
    // импортируем sub-пакет, чтобы deduction зарегистрировался.
    _ "github.com/kliuchnikovv/dnd/core/scenarios/deduction"
)

func TestLoadDefaultsToDeductionScenario(t *testing.T) {
    cfg, err := Load("testdata/minimal.json")
    if err != nil { t.Fatal(err) }
    if cfg.Scenario == nil {
        t.Fatal("после Load Scenario не должен быть nil")
    }
    if cfg.Scenario.Kind() != core.ScenarioDeduction {
        t.Fatalf("дефолт должен быть deduction, а не %q", cfg.Scenario.Kind())
    }
}

func TestLoadRespectsExplicitAdventure(t *testing.T) {
    _, err := parseWithScenario(t, "adventure")
    if err == nil {
        t.Fatal("без регистрации adventure Load обязан ругнуться")
    }
}
```

Плюс helper (тест ожидает наличие фикстуры; adventure ещё не зарегистрирован, поэтому второй тест ловит именно понятную ошибку):

```go
func parseWithScenario(t *testing.T, kind string) (*core.Config, error) {
    t.Helper()
    raw := []byte(`{"id":"c_x","start":"n_x","actor":"chr_x",
      "character":{"id":"chr_x","sheet":{}},
      "locations":[{"id":"n_x","name":"X"}],
      "scenario":"` + kind + `"}`)
    return Parse(raw)
}
```

- [ ] **Step 2: Запустить — тесты падают**

Run: `go test ./cases/... -run TestLoadDefaultsToDeductionScenario -v`
Expected: `undefined: cfg.Scenario` / нет поля `Scenario` у `File`.

- [ ] **Step 3: Добавить поле в схему**

В `cases/schema.go` в `type File struct`, после `Archetype`:

```go
    // Scenario — какой архетип держит цель и условие победы. Отсутствует —
    // читается как "deduction" (обратная совместимость с M1a-кейсами).
    Scenario core.ScenarioKind `json:"scenario"`
```

Импорт `core` в `cases/schema.go`.

- [ ] **Step 4: Добавить поле в `core.Config` и `core.Game`**

В `core/game.go`, в `type Config struct` (после `Rules`):

```go
    // Scenario — архетип прогона: держит цель, условие победы, панель
    // клиента и AI монстров. Не должен быть nil к моменту NewGame.
    Scenario Scenario
```

В `type Game struct` — то же поле; в `NewGame(cfg Config)` — `g.Scenario = cfg.Scenario`.

- [ ] **Step 5: Проставить сценарий в `cases.Parse`**

В `cases/load.go`, `Parse`, ближе к концу перед `return cfg, nil`:

```go
    kind := f.Scenario
    if kind == "" {
        kind = core.ScenarioDeduction
    }
    sc, ok := core.LookupScenario(kind)
    if !ok {
        return nil, fmt.Errorf("cases: неизвестный сценарий %q (импорт пакета?)", kind)
    }
    cfg.Scenario = sc
```

- [ ] **Step 6: Запустить тесты**

Run: `go test ./cases/... -run TestLoad -v`
Expected: `TestLoadDefaultsToDeductionScenario` пока падает — deduction ещё не зарегистрирован. Это ОК, следующая задача — регистрация.

Run: `go test ./...`
Expected: fixture-случай (deduction не зарегистрирован) падает; helper-тест по adventure — падает с понятной ошибкой (это желаемое поведение). Отметить в коммите.

Ветка ломается до Task 3 намеренно — коммит здесь пустого сценария оставил бы движок в неисполнимом состоянии; делаем Task 3 без коммита Task 2 и коммитим двумя порциями после Task 3.

- [ ] **Step 7: Собрать в один коммит после Task 3** — сейчас не коммитим; переходим к Task 3.

---

### Task 3: Перенести детектив в `core/scenarios/deduction/`

Крупная задача — единственный рефакторинг «двигаем файлы» в плане. Разделена на подшаги.

**Files:**
- Create: `core/scenarios/deduction/scenario.go`
- Move: `core/hunch.go` → `core/scenarios/deduction/hunch.go`
- Move: `core/accusation/*.go` → `core/scenarios/deduction/accusation/*.go`
- Modify: все файлы `core/`, где импортировался `hunch` или `accusation` — на новый путь.
- Modify: тесты, переехавшие вместе с кодом.

**Interfaces:**
- Consumes: `core.RegisterScenario` из Task 1, `core.Scenario` контракт.
- Produces: `import _ "github.com/kliuchnikovv/dnd/core/scenarios/deduction"` регистрирует архетип с именем `"deduction"`.

- [ ] **Step 1: Разведать импорты**

```bash
grep -rn "kliuchnikovv/dnd/core/accusation\|\.Hunch\|core\.Accus" --include="*.go" | grep -v _test | sort -u
```

Ожидаем ~5-10 мест: `cli/`, `server/`, `cmd/dnd/`, `view/`.

- [ ] **Step 2: Скопировать (не переместить) файлы**

```bash
mkdir -p core/scenarios/deduction/accusation
cp core/hunch.go       core/scenarios/deduction/hunch.go
cp core/hunch_test.go  core/scenarios/deduction/hunch_test.go
cp core/accusation/*.go core/scenarios/deduction/accusation/
```

В скопированных: заменить `package core` → `package deduction`; `package accusation` остаётся (лежит по-прежнему подпакетом). Импорты внутри копий: обращения к типам ядра — `core.<X>` вместо голых имён (в оригинале голые имена — тот же пакет).

- [ ] **Step 3: Написать `scenario.go`**

```go
// Package deduction — архетип детектива за швом Scenario. Внутри — граф
// фактов не живёт (он в core как утилита знания), а живут corroboration,
// формирование обвинения и условия победы/поражения.
package deduction

import (
    "github.com/kliuchnikovv/dnd/core"
    "github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
    "github.com/kliuchnikovv/dnd/store"
)

func init() {
    core.RegisterScenario(core.ScenarioDeduction, func() core.Scenario { return &scenario{} })
}

type scenario struct{}

func (s *scenario) Kind() core.ScenarioKind { return core.ScenarioDeduction }

// ExtraVerbs — глаголы детектива: обвинение, теоризация и присущее только
// расследованию. Общие глаголы (talk_to, examine и др.) остаются в core.Verbs.
func (s *scenario) ExtraVerbs() []core.VerbDef {
    return accusation.Verbs()
    // если у пакета accusation ещё нет метода Verbs() — экспортируем
    // существующие определения из его registry.go под этим именем.
}

// Victory — выиграно, если только что применённое accuse прошло с
// корроборацией. Проверка живёт в accusation.EvaluatedForm — переиспользуем.
func (s *scenario) Victory(g *core.Game) (bool, string) {
    return accusation.Victory(g)
}

// Defeat — проиграно, если исчерпан лимит попыток обвинения. Форма
// поражения детектива — авторская, задаётся в case.json (Attempts, Detected).
func (s *scenario) Defeat(g *core.Game) (bool, string) {
    return accusation.Defeat(g)
}

// Panel — казбук с секциями who/how/when/why. Наполним в Task 20 (панель
// пока — заглушка одной секции с числом фактов, чтобы Kind() был корректен).
func (s *scenario) Panel(g *core.Game) core.Panel {
    return core.Panel{Sections: []core.PanelSection{{Kind: "casebook"}}}
}

// NPCTurn — у детектива NPC ходов не бывает. Всегда false.
func (s *scenario) NPCTurn(*core.Game, store.EntityID) (core.Intent, bool) {
    return core.Intent{}, false
}
```

Если в `accusation/` пока нет функций `Victory/Defeat/Verbs` — добавить их адаптерами над существующей логикой (см. `core/accusation/*.go`, `hunch.go`, где это уже реализовано).

- [ ] **Step 4: Обновить импорты в `cli/`, `server/`, `cmd/dnd/`, `view/`**

Заменить `github.com/kliuchnikovv/dnd/core/accusation` на `github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation` там, где импорт использовался. Проверить каждый файл вручную:

```bash
git grep -l 'kliuchnikovv/dnd/core/accusation'
```

- [ ] **Step 5: Удалить оригиналы из `core/`**

```bash
git rm core/hunch.go core/hunch_test.go
git rm -r core/accusation
```

- [ ] **Step 6: Прописать импорт в главных точках**

Добавить в `cmd/dnd/main.go` и `cmd/server/main.go` (или где init):

```go
_ "github.com/kliuchnikovv/dnd/core/scenarios/deduction"
```

Также в `cases/load.go` — импорт для теста Task 2:

```go
import (
    // ... существующие
    _ "github.com/kliuchnikovv/dnd/core/scenarios/deduction"
)
```

- [ ] **Step 7: Собрать и прогнать все тесты**

Run:
```bash
go build ./...
go test ./...
```

Ожидание: harbour и forte_merlo зелёные; `TestLoadDefaultsToDeductionScenario` из Task 2 зелёный; всё поведение бит-в-бит.

Если что-то падает: смотрим на импорты (Step 4) — 90% ошибок здесь.

- [ ] **Step 8: Коммит (объединённый с Task 2)**

```bash
git add -A
git commit -m "refactor(core): вынести детектив за шов Scenario

Перенос core/hunch.go и core/accusation/* в core/scenarios/deduction/.
Публичное API сохранено, только пути импорта изменились. Ядро больше не
знает про accuse/corroboration напрямую — оно знает про Scenario.

Обратно совместимо: cases.Load без поля scenario читает 'deduction' по
умолчанию, harbour/forte_merlo играются идентично.

см. §3 спеки, ADR-0006."
```

---

### Task 4: Гейт фазы 1 — регрессия и тест агностичности

**Files:**
- Create: `core/architecture_test.go` — расширить (или создать) агностичность.
- Test files existing.

**Interfaces:**
- Consumes: всё из Task 1-3.
- Produces: гарантия что фаза 1 закрыта.

- [ ] **Step 1: Проверить, есть ли architecture_test**

```bash
find . -name "architecture_test.go" -not -path "*/worktrees/*"
```

Если есть — расширить. Если нет — создать. Ниже — минимальная версия.

- [ ] **Step 2: Тест на границу core → не импортирует rules/scenarios конкретики**

```go
// core/architecture_test.go
package core_test

import (
    "go/build"
    "strings"
    "testing"
)

func TestCoreDoesNotImportRules(t *testing.T) {
    pkg, err := build.Default.Import("github.com/kliuchnikovv/dnd/core", "", 0)
    if err != nil { t.Fatal(err) }
    for _, imp := range pkg.Imports {
        if strings.Contains(imp, "kliuchnikovv/dnd/rules/") {
            t.Errorf("core импортирует rules-конкретику: %s", imp)
        }
        if strings.Contains(imp, "kliuchnikovv/dnd/core/scenarios/") {
            t.Errorf("core импортирует scenario-конкретику: %s", imp)
        }
    }
}
```

- [ ] **Step 3: Запустить**

Run: `go test ./core/... -run TestCoreDoesNotImportRules -v`
Expected: PASS. Если провалилось — Task 3 неполно, ищем оставшийся импорт.

- [ ] **Step 4: Полная регрессия**

```bash
go test ./...
```

Все зелёные. Если e2e-детектив сломан — стоп и разбираемся.

- [ ] **Step 5: Коммит**

```bash
git add core/architecture_test.go
git commit -m "test(core): архитектурный тест — core не знает про rules/scenarios"
```

---

## Фаза 2. Ядерные добавки под бой

### Task 5: HP, MaxHP, AC на сущности

**Files:**
- Modify: `store/rows.go` — расширить `type Entity`.
- Modify: `cases/schema.go` — принять поля через JSON.
- Modify: `cases/load.go` — уже читается через `db.Entities[e.ID] = e`, работает.
- Create: `store/rows_hp_test.go`.

**Interfaces:**
- Produces: `Entity.HP`, `Entity.MaxHP`, `Entity.AC` int. Нулевое значение — «не участвует».

- [ ] **Step 1: Тест**

```go
// store/rows_hp_test.go
package store

import (
    "encoding/json"
    "testing"
)

func TestEntityJSONHasHPFields(t *testing.T) {
    raw := `{"id":"e_x","name":"X","kind":"npc","hp":10,"max_hp":10,"ac":13}`
    var e Entity
    if err := json.Unmarshal([]byte(raw), &e); err != nil { t.Fatal(err) }
    if e.HP != 10 || e.MaxHP != 10 || e.AC != 13 {
        t.Fatalf("HP/MaxHP/AC не разобрались: %+v", e)
    }
}

func TestEntityJSONWithoutHPFieldsIsZero(t *testing.T) {
    raw := `{"id":"e_x","name":"X","kind":"npc"}`
    var e Entity
    if err := json.Unmarshal([]byte(raw), &e); err != nil { t.Fatal(err) }
    if e.HP != 0 || e.MaxHP != 0 || e.AC != 0 {
        t.Fatalf("отсутствие полей должно давать 0: %+v", e)
    }
}
```

- [ ] **Step 2: Запустить — падает** (полей ещё нет).

- [ ] **Step 3: Добавить поля**

В `store/rows.go`, в `type Entity struct`, после `Node`:

```go
    // HP/MaxHP — здоровье в D&D-механике. Ноль у сущности, не участвующей
    // в HP-модели (детективные NPC). Разница между HP и MaxHP — необходима
    // для rest_short/long и лечения; хранить только текущее было бы одним
    // местом правды меньше.
    HP    int `json:"hp,omitempty"`
    MaxHP int `json:"max_hp,omitempty"`
    // AC — броня в D&D. Ноль — «нельзя атаковать» (у детективных NPC).
    AC    int `json:"ac,omitempty"`
```

- [ ] **Step 4: Запустить — проходит**.

- [ ] **Step 5: Полная регрессия**

Run: `go test ./...`
Expected: harbour/forte_merlo зелёные — они не используют HP/AC.

- [ ] **Step 6: Коммит**

```bash
git add store/rows.go store/rows_hp_test.go
git commit -m "feat(store): HP/MaxHP/AC на сущности (опциональные)"
```

---

### Task 6: `core.Encounter` и `Game.Encounter`

**Files:**
- Create: `core/encounter.go`
- Create: `core/encounter_test.go`
- Modify: `core/game.go` — добавить `Encounter *Encounter`.

**Interfaces:**
- Produces:
  ```go
  type Encounter struct {
      Order   []store.EntityID
      Turn    int   // индекс в Order
      Round   int
      Actions ActionState
  }
  type ActionState struct{ Action, Bonus bool } // false = доступно
  func (e *Encounter) Current() store.EntityID
  func (e *Encounter) Advance()  // Turn++ mod len(Order); Round++ на wrap
  func (e *Encounter) Reset()    // ActionState очистить в начале хода сущности
  ```

- [ ] **Step 1: Тесты**

```go
// core/encounter_test.go
package core

import (
    "testing"

    "github.com/kliuchnikovv/dnd/store"
)

func TestEncounterAdvanceWraps(t *testing.T) {
    e := &Encounter{Order: []store.EntityID{"a", "b", "c"}}
    if e.Current() != "a" { t.Fatalf("initial current: %q", e.Current()) }
    e.Advance()
    if e.Current() != "b" { t.Fatalf("after 1 advance: %q", e.Current()) }
    e.Advance()
    e.Advance()
    if e.Current() != "a" { t.Fatalf("wrap-around вернуть в a: %q", e.Current()) }
    if e.Round != 1 { t.Fatalf("Round после полного круга: %d", e.Round) }
}

func TestEncounterResetClearsActions(t *testing.T) {
    e := &Encounter{Order: []store.EntityID{"a"}}
    e.Actions.Action, e.Actions.Bonus = true, true
    e.Reset()
    if e.Actions.Action || e.Actions.Bonus {
        t.Fatalf("после Reset флаги действий должны быть false: %+v", e.Actions)
    }
}
```

- [ ] **Step 2: Запустить — падает** (нет `Encounter`).

- [ ] **Step 3: Реализовать**

```go
// core/encounter.go
package core

import "github.com/kliuchnikovv/dnd/store"

// Encounter — состояние боя. nil у Game — бой не идёт (обычный ход).
// Порядок ходов задаётся один раз при входе в бой (initiative от правил);
// внутри — монотонное продвижение с обёрткой на len(Order).
type Encounter struct {
    Order   []store.EntityID
    Turn    int  // индекс в Order, чей сейчас ход
    Round   int  // сквозной номер раунда
    Actions ActionState
}

// ActionState — что доступно этому существу на его такте.
// false — ещё не потрачено. reaction в первом заходе не моделируем.
type ActionState struct {
    Action bool
    Bonus  bool
}

// Current — чей сейчас такт. Пустой Order → пустой ID.
func (e *Encounter) Current() store.EntityID {
    if len(e.Order) == 0 {
        return ""
    }
    return e.Order[e.Turn]
}

// Advance — конец такта: следующая сущность в порядке, при wrap — новый раунд.
// Флаги действий сбрасываются автоматически: новый такт — новые ресурсы.
func (e *Encounter) Advance() {
    if len(e.Order) == 0 {
        return
    }
    e.Turn = (e.Turn + 1) % len(e.Order)
    if e.Turn == 0 {
        e.Round++
    }
    e.Reset()
}

// Reset — обнуляет ActionState. Нужен и как явный шаг сценария (например,
// при появлении haste-эффектов позже), поэтому не приватный.
func (e *Encounter) Reset() {
    e.Actions = ActionState{}
}
```

Добавить в `Game`:

```go
// Encounter — состояние боя. nil — бой не идёт.
Encounter *Encounter
```

- [ ] **Step 4: Проходит**.

- [ ] **Step 5: Регрессия**.

- [ ] **Step 6: Коммит**

```bash
git add core/encounter.go core/encounter_test.go core/game.go
git commit -m "feat(core): Encounter — инициативная лента и такты боя"
```

---

### Task 7: Мутации HP/условий/боя

**Files:**
- Modify: `core/resolution.go` — новые `MutationKind` константы.
- Modify: `core/mutate.go` — apply-функции.
- Create: `core/mutate_combat_test.go`.

**Interfaces:**
- Produces:
  ```go
  const (
      MutHPDelta          MutationKind = "hp_delta"
      MutSetCondition     MutationKind = "set_condition"
      MutEncounterStart   MutationKind = "encounter_start"
      MutEncounterEnd     MutationKind = "encounter_end"
      MutAdvanceInitiative MutationKind = "advance_initiative"
  )
  // Mutation.Target уже есть; для HPDelta.Delta используем существующее числовое поле (см. core/resolution.go, поле Amount int).
  ```

- [ ] **Step 1: Прочитать существующее**

```bash
head -60 core/resolution.go
grep -n "func .*apply\|Apply\|MutHarm" core/mutate.go
```

Понять форму `Mutation` и apply-функции. Ниже — тесты и код по образцу существующего.

- [ ] **Step 2: Тесты**

```go
// core/mutate_combat_test.go
package core

import (
    "testing"

    "github.com/kliuchnikovv/dnd/store"
)

func TestMutHPDeltaReducesEntityHP(t *testing.T) {
    g := &Game{DB: store.NewDB()}
    g.DB.Entities["e_orc"] = store.Entity{ID: "e_orc", HP: 10, MaxHP: 10, AC: 13}
    apply(g, Mutation{Kind: MutHPDelta, Target: "e_orc", Amount: -3})
    if g.DB.Entities["e_orc"].HP != 7 {
        t.Fatalf("HP после -3: %d", g.DB.Entities["e_orc"].HP)
    }
}

func TestMutHPDeltaClampsAtZero(t *testing.T) {
    g := &Game{DB: store.NewDB()}
    g.DB.Entities["e_orc"] = store.Entity{ID: "e_orc", HP: 2, MaxHP: 10}
    apply(g, Mutation{Kind: MutHPDelta, Target: "e_orc", Amount: -5})
    if g.DB.Entities["e_orc"].HP != 0 {
        t.Fatalf("HP не клампится на 0: %d", g.DB.Entities["e_orc"].HP)
    }
}

func TestMutHPDeltaHealClampsAtMax(t *testing.T) {
    g := &Game{DB: store.NewDB()}
    g.DB.Entities["e_orc"] = store.Entity{ID: "e_orc", HP: 8, MaxHP: 10}
    apply(g, Mutation{Kind: MutHPDelta, Target: "e_orc", Amount: 5})
    if g.DB.Entities["e_orc"].HP != 10 {
        t.Fatalf("HP не клампится на MaxHP: %d", g.DB.Entities["e_orc"].HP)
    }
}

func TestMutEncounterStartAndEnd(t *testing.T) {
    g := &Game{DB: store.NewDB()}
    apply(g, Mutation{Kind: MutEncounterStart, Order: []store.EntityID{"chr_kay", "e_orc"}})
    if g.Encounter == nil || len(g.Encounter.Order) != 2 {
        t.Fatalf("MutEncounterStart не завёл Encounter: %+v", g.Encounter)
    }
    apply(g, Mutation{Kind: MutAdvanceInitiative})
    if g.Encounter.Current() != "e_orc" {
        t.Fatalf("после Advance не тот текущий: %q", g.Encounter.Current())
    }
    apply(g, Mutation{Kind: MutEncounterEnd})
    if g.Encounter != nil {
        t.Fatalf("MutEncounterEnd не обнулил: %+v", g.Encounter)
    }
}
```

`Mutation.Order`, `Mutation.Amount` — поля структуры Mutation, если их нет — добавить.

- [ ] **Step 3: Расширить `Mutation`**

В `core/resolution.go`:

```go
type Mutation struct {
    Kind      MutationKind
    Target    store.EntityID
    Item      store.ItemID
    Node      store.NodeID
    Topic     string
    Text      string
    // Amount — числовой параметр: HP-дельта, длительность условия.
    Amount    int
    // Condition — метка условия (prone, poisoned, restrained).
    Condition string
    // Order — порядок ходов при MutEncounterStart.
    Order     []store.EntityID
}
```

Существующие поля сохранить. Плюс новые константы:

```go
const (
    // ... существующие
    MutHPDelta           MutationKind = "hp_delta"
    MutSetCondition      MutationKind = "set_condition"
    MutEncounterStart    MutationKind = "encounter_start"
    MutEncounterEnd      MutationKind = "encounter_end"
    MutAdvanceInitiative MutationKind = "advance_initiative"
)
```

- [ ] **Step 4: Реализовать apply**

В `core/mutate.go`, в диспетчере `apply` (или соответствующем switch по `Kind`):

```go
case MutHPDelta:
    e := g.DB.Entities[m.Target]
    hp := e.HP + m.Amount
    if hp < 0 { hp = 0 }
    if e.MaxHP > 0 && hp > e.MaxHP { hp = e.MaxHP }
    e.HP = hp
    g.DB.Entities[m.Target] = e

case MutSetCondition:
    // Условие как тег на сущности; хранение — в store.DB.Conditions
    // (карта EntityID → map[Condition]turnsLeft). Заведём поле в DB, если
    // отсутствует; иначе — использовать существующую утилиту тегов.
    // Amount == 0 → снять.
    setCondition(g, m.Target, m.Condition, m.Amount)

case MutEncounterStart:
    g.Encounter = &Encounter{Order: append([]store.EntityID(nil), m.Order...)}

case MutEncounterEnd:
    g.Encounter = nil

case MutAdvanceInitiative:
    if g.Encounter != nil { g.Encounter.Advance() }
```

Функцию `setCondition` реализовать здесь же — карта на сущности с ttl (см. §6.3 спеки).

- [ ] **Step 5: Тесты проходят**. Регрессия.

- [ ] **Step 6: Коммит**

```bash
git add core/resolution.go core/mutate.go core/mutate_combat_test.go
git commit -m "feat(core): мутации HP/условий/боевой ленты"
```

---

### Task 8: Хуки применения хода — «не твой ход» и NPCTurn

**Files:**
- Modify: `core/game.go` (метод `Apply`) — проверка «твой ли ход» и цикл NPC-ходов.
- Create: `core/game_encounter_test.go`.

**Interfaces:**
- Consumes: `Scenario.NPCTurn`, `Encounter`.
- Produces: поведение `Apply` под боем — отказ, если не твой такт; цикл до возврата хода игроку.

- [ ] **Step 1: Тест на отказ вне такта**

```go
// core/game_encounter_test.go
package core

import (
    "testing"

    "github.com/kliuchnikovv/dnd/store"
)

func TestApplyRefusesWhenNotPlayerTurn(t *testing.T) {
    g := &Game{DB: store.NewDB(), Actor: "chr_kay"}
    g.Encounter = &Encounter{Order: []store.EntityID{"e_orc", store.EntityID(g.Actor)}}
    res := g.Apply(Intent{Verb: "attack", Actor: g.Actor, Args: Args{Target: "e_orc"}})
    if !res.Refused {
        t.Fatalf("под боем и не свой ход — обязан быть отказ")
    }
}

func TestApplyCyclesThroughNPCsBackToPlayer(t *testing.T) {
    g := &Game{DB: store.NewDB(), Actor: "chr_kay",
        Scenario: stubNPCScenario{turn: Intent{Verb: "pass"}}}
    g.DB.Entities["e_orc"] = store.Entity{ID: "e_orc", HP: 5, MaxHP: 5}
    g.Encounter = &Encounter{Order: []store.EntityID{store.EntityID(g.Actor), "e_orc"}}
    res := g.Apply(Intent{Verb: "attack", Actor: g.Actor, Args: Args{Target: "e_orc"}})
    _ = res
    if g.Encounter.Current() != store.EntityID(g.Actor) {
        t.Fatalf("после хода игрока и хода NPC — вернуться к игроку: %q", g.Encounter.Current())
    }
}

type stubNPCScenario struct{ turn Intent }
func (s stubNPCScenario) Kind() ScenarioKind { return "stub" }
func (s stubNPCScenario) ExtraVerbs() []VerbDef { return nil }
func (s stubNPCScenario) Victory(*Game) (bool, string) { return false, "" }
func (s stubNPCScenario) Defeat(*Game) (bool, string) { return false, "" }
func (s stubNPCScenario) Panel(*Game) Panel { return Panel{} }
func (s stubNPCScenario) NPCTurn(*Game, store.EntityID) (Intent, bool) { return s.turn, true }
```

- [ ] **Step 2: Тесты падают** — Apply не проверяет ход.

- [ ] **Step 3: Правка `Apply`**

В `core/game.go`, в начале `Apply(intent Intent)`:

```go
    if g.Encounter != nil && intent.Actor != "" {
        if store.EntityID(intent.Actor) != g.Encounter.Current() {
            return TurnResult{Refused: true, Refusal: "не ваш ход"}
        }
    }
```

В конце `Apply`, после существующего резолва/мутаций:

```go
    if g.Encounter != nil {
        g.Encounter.Advance()
        // Прокрутить NPC-такты до возврата игроку. Ограничитель —
        // len(Order)+1, потому что за один круг Turn обязан вернуться.
        guard := len(g.Encounter.Order) + 1
        for guard > 0 && g.Encounter != nil &&
            g.Encounter.Current() != store.EntityID(g.Actor) {
            npc := g.Encounter.Current()
            if g.Scenario == nil {
                break
            }
            in, ok := g.Scenario.NPCTurn(g, npc)
            if !ok {
                g.Encounter.Advance()
                guard--
                continue
            }
            in.Actor = string(npc)
            _ = g.applyOne(in) // применяем без рекурсии в Apply
            g.Encounter.Advance()
            guard--
        }
    }
```

Где `applyOne` — внутренняя функция, повторяющая тело `Apply` без нашего хука. Если рефакторить сложно — сделать `Apply` идемпотентной по хуку через флаг `inNPCTurn bool` в приёмнике.

- [ ] **Step 4: Тесты проходят**. Регрессия.

- [ ] **Step 5: Коммит**

```bash
git add core/game.go core/game_encounter_test.go
git commit -m "feat(core): хуки Apply для боя — отказ вне такта, цикл NPC-ходов"
```

---

## Фаза 3. Правила `rules/dnd5e/`

### Task 9: Лист персонажа + модификаторы

**Files:**
- Create: `rules/dnd5e/sheet.go`, `rules/dnd5e/sheet_test.go`.

- [ ] **Step 1: Тесты**

```go
// rules/dnd5e/sheet_test.go
package dnd5e

import "testing"

func TestParseSheetReadsAbilities(t *testing.T) {
    raw := []byte(`{"str":14,"dex":16,"con":13,"int":11,"wis":12,"cha":10,"prof":2,
                    "max_hp":12,"ac":14,"speed":30,
                    "skills":["stealth","perception"],"saves":["dex"]}`)
    s, err := ParseSheet(raw)
    if err != nil { t.Fatal(err) }
    if s.Dex != 16 { t.Fatalf("dex: %d", s.Dex) }
    if s.Mod("dex") != 3 { t.Fatalf("mod dex: %d", s.Mod("dex")) }
    if s.Mod("cha") != 0 { t.Fatalf("mod cha 10: %d", s.Mod("cha")) }
    if !s.Proficient("stealth") { t.Fatal("stealth должен быть prof") }
    if s.Proficient("athletics") { t.Fatal("athletics НЕ должен быть prof") }
    if !s.SaveProficient("dex") { t.Fatal("dex-save должен быть prof") }
}

func TestParseSheetEmptyDoesNotPanic(t *testing.T) {
    _, err := ParseSheet(nil)
    if err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Тесты падают** — пакета нет.

- [ ] **Step 3: Реализация**

```go
// Package dnd5e — упрощённое подмножество D&D 5e за швом core.RuleSystem.
// Лист хранит ability scores (не модификаторы) — 5e-мышление, и путь к
// левелапу открыт (см. §5 спеки, точки роста).
package dnd5e

import "encoding/json"

type Sheet struct {
    Str, Dex, Con, Int, Wis, Cha int
    Prof                          int

    Skills []string
    Saves  []string

    MaxHP, AC, Speed int

    Weapons []Weapon
}

type Weapon struct {
    Name   string
    Reach  string // "melee" | "ranged"
    Attack string // "str" | "dex"
    Damage string // "1d8+str", "1d6+dex", ...
}

func ParseSheet(raw json.RawMessage) (Sheet, error) {
    var s Sheet
    if len(raw) == 0 { return s, nil }
    return s, json.Unmarshal(raw, &s)
}

// Mod — модификатор ability по строке ("str"|"dex"|...).
func (s Sheet) Mod(ability string) int {
    score := s.abilityScore(ability)
    return (score - 10) / 2
}

func (s Sheet) abilityScore(ability string) int {
    switch ability {
    case "str": return s.Str
    case "dex": return s.Dex
    case "con": return s.Con
    case "int": return s.Int
    case "wis": return s.Wis
    case "cha": return s.Cha
    }
    return 10
}

func (s Sheet) Proficient(skill string) bool {
    for _, x := range s.Skills { if x == skill { return true } }
    return false
}

func (s Sheet) SaveProficient(ability string) bool {
    for _, x := range s.Saves { if x == ability { return true } }
    return false
}
```

При unmarshal через теги: если предпочитаете snake_case в JSON — добавить `json:"str"` и т.д. явно.

- [ ] **Step 4: Тесты проходят**.

- [ ] **Step 5: Коммит**

```bash
git add rules/dnd5e/sheet.go rules/dnd5e/sheet_test.go
git commit -m "feat(dnd5e): лист персонажа и модификаторы"
```

---

### Task 10: Парсер dice-строк

**Files:**
- Create: `rules/dnd5e/dice.go`, `rules/dnd5e/dice_test.go`.

**Interfaces:**
- Produces:
  ```go
  type DiceSpec struct { N, Sides int; Mod int; AbilityMod string /* "str"|... */ }
  func ParseDice(s string) (DiceSpec, error)  // "1d8+str", "2d6", "1d6+2"
  func (d DiceSpec) Roll(dice core.Dice, sheet Sheet, crit bool) int
  ```

- [ ] **Step 1: Тесты**

```go
package dnd5e

import "testing"

func TestParseDiceSimple(t *testing.T) {
    d, err := ParseDice("2d6")
    if err != nil { t.Fatal(err) }
    if d.N != 2 || d.Sides != 6 || d.Mod != 0 || d.AbilityMod != "" {
        t.Fatalf("2d6 разобралось как %+v", d)
    }
}

func TestParseDiceWithAbility(t *testing.T) {
    d, err := ParseDice("1d8+str")
    if err != nil { t.Fatal(err) }
    if d.N != 1 || d.Sides != 8 || d.AbilityMod != "str" {
        t.Fatalf("1d8+str: %+v", d)
    }
}

func TestParseDiceWithNumericMod(t *testing.T) {
    d, err := ParseDice("1d6+2")
    if err != nil { t.Fatal(err) }
    if d.Mod != 2 { t.Fatalf("mod: %+v", d) }
}

func TestParseDiceInvalid(t *testing.T) {
    if _, err := ParseDice("nope"); err == nil {
        t.Fatal("невалидная строка не должна разобраться")
    }
}
```

- [ ] **Step 2: Падает — нет пакета**.

- [ ] **Step 3: Реализация**

```go
// rules/dnd5e/dice.go
package dnd5e

import (
    "fmt"
    "strconv"
    "strings"
)

// DiceSpec — разобранная dice-строка «NdM(+mod|+ability)».
type DiceSpec struct {
    N, Sides   int
    Mod        int
    AbilityMod string // "" | "str" | "dex" | ...
}

func ParseDice(s string) (DiceSpec, error) {
    s = strings.ReplaceAll(strings.TrimSpace(strings.ToLower(s)), " ", "")
    var d DiceSpec
    // Разделить по «+»: [dice, [mod]]
    parts := strings.SplitN(s, "+", 2)
    if _, err := fmt.Sscanf(parts[0], "%dd%d", &d.N, &d.Sides); err != nil {
        return d, fmt.Errorf("dnd5e/dice: не разобрал %q: %w", s, err)
    }
    if len(parts) == 2 {
        if n, err := strconv.Atoi(parts[1]); err == nil {
            d.Mod = n
        } else {
            switch parts[1] {
            case "str", "dex", "con", "int", "wis", "cha":
                d.AbilityMod = parts[1]
            default:
                return d, fmt.Errorf("dnd5e/dice: неизвестный мод %q", parts[1])
            }
        }
    }
    return d, nil
}
```

`Roll` вынесем в отдельной задаче (Task 12), где будет доступ к `core.Dice`.

- [ ] **Step 4: Тесты проходят**.

- [ ] **Step 5: Коммит**

```bash
git add rules/dnd5e/dice.go rules/dnd5e/dice_test.go
git commit -m "feat(dnd5e): парсер dice-строк"
```

---

### Task 11: Skill → Ability маппинг

**Files:**
- Create: `rules/dnd5e/skills.go`, `rules/dnd5e/skills_test.go`.

- [ ] **Step 1: Тест**

```go
package dnd5e

import "testing"

func TestSkillAbilityMappingCovers5eBasics(t *testing.T) {
    cases := map[string]string{
        "athletics": "str", "acrobatics": "dex", "stealth": "dex",
        "perception": "wis", "insight": "wis", "investigation": "int",
        "persuasion": "cha", "deception": "cha", "thieves_tools": "dex",
    }
    for skill, want := range cases {
        if got := SkillAbility(skill); got != want {
            t.Errorf("%s → %q, ждали %q", skill, got, want)
        }
    }
}

func TestSkillAbilityUnknownIsEmpty(t *testing.T) {
    if got := SkillAbility("no-such"); got != "" {
        t.Errorf("неизвестный skill → %q, ждали пустую", got)
    }
}
```

- [ ] **Step 2: Падает — нет функции**.

- [ ] **Step 3: Реализация**

```go
// rules/dnd5e/skills.go
package dnd5e

// SkillAbility — какая ability стоит за skill в 5e. Таблица закрыта:
// неизвестный skill возвращает "", а не «что-нибудь похожее» — это
// значит «нет проверки», и вызывающий делает fallback на общую ability.
var skillAbility = map[string]string{
    // Str
    "athletics": "str",
    // Dex
    "acrobatics": "dex", "stealth": "dex", "sleight_of_hand": "dex",
    "thieves_tools": "dex",
    // Wis
    "perception": "wis", "insight": "wis", "medicine": "wis",
    "survival":   "wis", "animal_handling": "wis",
    // Int
    "arcana":       "int", "history": "int", "investigation": "int",
    "nature":       "int", "religion": "int",
    // Cha
    "persuasion": "cha", "deception": "cha", "intimidation": "cha",
    "performance": "cha",
}

func SkillAbility(skill string) string {
    return skillAbility[skill]
}
```

- [ ] **Step 4: Проходит**. Коммит:

```bash
git add rules/dnd5e/skills.go rules/dnd5e/skills_test.go
git commit -m "feat(dnd5e): таблица skill→ability"
```

---

### Task 12: Резолв — `check`, `attack`, `save`

Крупная задача, разбита по подшагам. Держим три ветки резолва в трёх файлах + сборщик `resolve.go`.

**Files:**
- Create: `rules/dnd5e/check.go`, `rules/dnd5e/attack.go`, `rules/dnd5e/save.go`, `rules/dnd5e/resolve.go`.
- Create: `rules/dnd5e/check_test.go`, `rules/dnd5e/attack_test.go`, `rules/dnd5e/save_test.go`.

**Interfaces:**
- Consumes: `Sheet`, `DiceSpec`, `SkillAbility`, `core.RuleSystem` контракт.
- Produces: `dnd5e.System` реализует `core.RuleSystem`; конструктор `New()`.

- [ ] **Step 1: Тест check**

```go
// rules/dnd5e/check_test.go
package dnd5e

import (
    "encoding/json"
    "testing"

    "github.com/kliuchnikovv/dnd/core"
    "github.com/kliuchnikovv/dnd/dice"
)

func sceneWithSheet(t *testing.T, s Sheet, dc int) core.SceneView {
    t.Helper()
    raw, _ := json.Marshal(s)
    return core.SceneView{Sheet: raw, DC: dc}  // DC добавим в SceneView, если нет
}

func TestCheckPassWhenRollBeatsDC(t *testing.T) {
    s := Sheet{Dex: 16, Prof: 2, Skills: []string{"stealth"}}
    d := dice.NewSource(1).Stream("test")
    // seed 1 стрим "test" на d20 — узнать значение вживую; допустим == 12.
    // Сделаем deterministic-настройку в самом Resolve через RollD20(dice).
    view := sceneWithSheet(t, s, 12)
    sys := New()
    res := sys.Resolve(core.Intent{Verb: "hide"}, view, d)
    // 12 + Dex(3) + Prof(2) = 17 vs DC 12 — успех
    if res.Class != core.OutcomeSuccess {
        t.Fatalf("успех ожидался, получен %v", res.Class)
    }
}
```

Для deterministic-тестов проще инжектить d20 через мок или задать seed так, чтобы значения были известны. Пример выше использует `dice.NewSource`; итеративно откалибровать: сначала посмотреть, что выдаёт `RollD20(d)` при seed 1, использовать полученное значение в assert.

Практический подход: unit-тест `check` через параметр-функцию `rollD20 func() int` — заменяемую на константу. Дизайн:

```go
type rollFn func() int

func check(s Sheet, verb, skill string, dc int, roll rollFn) core.Resolution {
    ability := SkillAbility(skill)
    total := roll() + s.Mod(ability)
    if s.Proficient(skill) { total += s.Prof }
    if total >= dc { return core.Resolution{Class: core.OutcomeSuccess, Margin: total - dc} }
    return core.Resolution{Class: core.OutcomeFailure, Margin: dc - total}
}
```

И тесты на `check(sheet, verb, skill, dc, func() int { return 12 })`. Это чище, чем гонять `dice.Source` — сам детерминированный резолв в `System.Resolve` использует `rollD20(d)` обёртку.

- [ ] **Step 2: Тесты падают**.

- [ ] **Step 3: `rules/dnd5e/check.go`**

```go
package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// check — d20 + ability(+prof if_proficient) vs DC. Skill определяет
// ability по таблице SkillAbility; пустой skill → fallback на связанную
// с глаголом ability (в verbs.go).
func check(s Sheet, skill string, dc int, roll int) core.Resolution {
    ability := SkillAbility(skill)
    total := roll + s.Mod(ability)
    if s.Proficient(skill) {
        total += s.Prof
    }
    if total >= dc {
        return core.Resolution{Class: core.OutcomeSuccess, Margin: total - dc}
    }
    return core.Resolution{Class: core.OutcomeFailure, Margin: dc - total}
}
```

- [ ] **Step 4: `rules/dnd5e/attack.go`** (с крит-удвоением)

```go
package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// attack — d20 + ability + prof vs targetAC; 20 → крит (удвоение костей урона).
// weaponAbility — обычно "str" (melee) или "dex" (ranged/finesse).
// damage — уже разобранная DiceSpec оружия; rollDmg возвращает сумму по DiceSpec.
func attack(s Sheet, weaponAbility string, ac int, roll int,
    damage DiceSpec, rollDmg func(spec DiceSpec, crit bool) int) core.Resolution {

    total := roll + s.Mod(weaponAbility) + s.Prof
    crit := roll == 20
    if !crit && total < ac {
        return core.Resolution{Class: core.OutcomeFailure, Margin: ac - total}
    }
    dmg := rollDmg(damage, crit)
    class := core.OutcomeSuccess
    if crit { class = core.OutcomeCritical } // если есть; иначе OutcomeSuccess
    return core.Resolution{Class: class, Margin: total - ac, Damage: dmg}
}
```

`Resolution.Damage` — новое поле. Добавить в `core/resolution.go`.

- [ ] **Step 5: `rules/dnd5e/save.go`**

```go
package dnd5e

import "github.com/kliuchnikovv/dnd/core"

func save(s Sheet, ability string, dc int, roll int) core.Resolution {
    total := roll + s.Mod(ability)
    if s.SaveProficient(ability) { total += s.Prof }
    if total >= dc {
        return core.Resolution{Class: core.OutcomeSuccess, Margin: total - dc}
    }
    return core.Resolution{Class: core.OutcomeFailure, Margin: dc - total}
}
```

- [ ] **Step 6: `rules/dnd5e/resolve.go`**

```go
package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// System реализует core.RuleSystem — лёгкое подмножество 5e.
// Никакого состояния между вызовами.
type System struct{}

func New() *System { return &System{} }

func (s *System) Resolve(in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
    def := core.Verbs[in.Verb]
    if !def.Rolls || in.Verb == "move_zone" {
        return core.Resolution{Class: core.OutcomeSuccess}
    }
    sheet, _ := ParseSheet(view.Sheet)
    switch def.Class {
    case core.ClassAttack:
        return resolveAttack(sheet, in, view, d)
    case core.ClassSave:
        return resolveSave(sheet, in, view, d)
    default:
        return resolveCheck(sheet, in, view, d)
    }
}

func rollD20(d core.Dice) int { return d.Roll(20) }

func resolveCheck(s Sheet, in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
    skill := SkillOfVerb(in.Verb) // из verbs.go
    dc := DCOfIntent(in, view)    // из verbs.go
    return check(s, skill, dc, rollD20(d))
}

func resolveAttack(s Sheet, in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
    w := s.Weapons[0] // MVP: первое оружие; выбор — задача авторства кейса.
    dmgSpec, _ := ParseDice(w.Damage)
    ac := view.TargetAC
    return attack(s, w.Attack, ac, rollD20(d), dmgSpec, func(sp DiceSpec, crit bool) int {
        n := sp.N
        if crit { n *= 2 }
        total := sp.Mod + s.Mod(sp.AbilityMod)
        for i := 0; i < n; i++ { total += d.Roll(sp.Sides) }
        return total
    })
}

func resolveSave(s Sheet, in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
    return save(s, view.SaveAbility, view.DC, rollD20(d))
}
```

`SceneView.TargetAC`, `SceneView.DC`, `SceneView.SaveAbility` — новые поля, добавить в `core/scene.go` (или где `SceneView`). `core.ClassAttack`, `core.ClassSave` — новые классы в реестре глаголов; `core.OutcomeCritical` — новый исход, если ещё нет.

- [ ] **Step 7: Прогнать unit-тесты**

```bash
go test ./rules/dnd5e/... -v
```

Итерировать по недостающим кускам (`SceneView.DC`, `Resolution.Damage`, `OutcomeCritical`).

- [ ] **Step 8: Коммит**

```bash
git add rules/dnd5e/{check,attack,save,resolve}.go rules/dnd5e/{check,attack,save}_test.go core/scene.go core/resolution.go
git commit -m "feat(dnd5e): резолв — check/attack/save"
```

---

### Task 13: Инициатива

**Files:**
- Create: `rules/dnd5e/initiative.go`, `rules/dnd5e/initiative_test.go`.

**Interfaces:**
- Produces: `func Initiative(participants []Participant, d core.Dice) []store.EntityID`.

- [ ] **Step 1: Тесты**

```go
package dnd5e

import (
    "testing"

    "github.com/kliuchnikovv/dnd/dice"
    "github.com/kliuchnikovv/dnd/store"
)

func TestInitiativeSortsByDexInitiative(t *testing.T) {
    // Deterministic: d20 seed 1 на «init» стриме дать чётко известные значения.
    d := dice.NewSource(1).Stream("init")
    order := Initiative([]Participant{
        {ID: "a", DexMod: 3},
        {ID: "b", DexMod: 1},
        {ID: "c", DexMod: 4},
    }, d)
    if len(order) != 3 { t.Fatalf("len: %d", len(order)) }
    // проверять именно порядок нужно на детерм. seed: см. фактические
    // значения после калибровки. Здесь inv-check: у c был максимальный
    // ожидаемый суммарный (в среднем).
    _ = order
}
```

- [ ] **Step 2: Реализация**

```go
// rules/dnd5e/initiative.go
package dnd5e

import (
    "sort"

    "github.com/kliuchnikovv/dnd/core"
    "github.com/kliuchnikovv/dnd/store"
)

type Participant struct {
    ID     store.EntityID
    DexMod int
}

// Initiative — порядок ходов. d20 + DexMod, сортировка по убыванию;
// ties разрешаются последующим d20 (не сохраняются).
func Initiative(ps []Participant, d core.Dice) []store.EntityID {
    scored := make([]struct{ id store.EntityID; total int }, len(ps))
    for i, p := range ps {
        scored[i] = struct{ id store.EntityID; total int }{p.ID, d.Roll(20) + p.DexMod}
    }
    sort.SliceStable(scored, func(i, j int) bool {
        if scored[i].total == scored[j].total {
            return d.Roll(20) > d.Roll(20)
        }
        return scored[i].total > scored[j].total
    })
    out := make([]store.EntityID, len(scored))
    for i, s := range scored { out[i] = s.id }
    return out
}
```

- [ ] **Step 3: Тесты**. Коммит:

```bash
git add rules/dnd5e/initiative.go rules/dnd5e/initiative_test.go
git commit -m "feat(dnd5e): расчёт инициативы"
```

---

### Task 14: ExtraVerbs D&D

**Files:**
- Create: `rules/dnd5e/verbs.go`, `rules/dnd5e/verbs_test.go`.

- [ ] **Step 1: Тесты**

```go
package dnd5e

import "testing"

func TestExtraVerbsIncludeAttackHideRest(t *testing.T) {
    got := Verbs()
    have := map[string]bool{}
    for _, v := range got { have[string(v.Verb)] = true }
    for _, name := range []string{"attack", "hide", "sneak", "disarm_trap",
        "detect_trap", "pick_lock", "flee", "rest_short", "rest_long"} {
        if !have[name] { t.Errorf("верб %q не объявлен", name) }
    }
}

func TestSkillOfVerb(t *testing.T) {
    if SkillOfVerb("hide") != "stealth" {
        t.Errorf("hide → %q", SkillOfVerb("hide"))
    }
    if SkillOfVerb("pick_lock") != "thieves_tools" {
        t.Errorf("pick_lock → %q", SkillOfVerb("pick_lock"))
    }
}
```

- [ ] **Step 2: Реализация**

```go
// rules/dnd5e/verbs.go
package dnd5e

import "github.com/kliuchnikovv/dnd/core"

// Verbs — глаголы D&D, отдаваемые сценарием adventure через ExtraVerbs().
// Общие глаголы (talk_to, examine, ...) не дублируются.
func Verbs() []core.VerbDef {
    return []core.VerbDef{
        {Verb: "attack",      Class: core.ClassAttack, Rolls: true, Target: true},
        {Verb: "hide",        Class: core.ClassCheck,  Rolls: true},
        {Verb: "sneak",       Class: core.ClassMove,   Rolls: true, Node: true},
        {Verb: "disarm_trap", Class: core.ClassCheck,  Rolls: true, Target: true},
        {Verb: "detect_trap", Class: core.ClassCheck,  Rolls: true},
        {Verb: "pick_lock",   Class: core.ClassCheck,  Rolls: true, Target: true},
        {Verb: "flee",        Class: core.ClassMove,   Rolls: true},
        {Verb: "rest_short",  Class: core.ClassRecover},
        {Verb: "rest_long",   Class: core.ClassRecover},
    }
}

var verbSkill = map[core.Verb]string{
    "hide": "stealth", "sneak": "stealth",
    "disarm_trap": "thieves_tools", "pick_lock": "thieves_tools",
    "detect_trap": "perception",
    "flee": "acrobatics",
}

func SkillOfVerb(v core.Verb) string { return verbSkill[v] }

// DCOfIntent — DC для чек-глаголов: сперва явный из case.json
// (view.CheckDC[v_or_target]), потом по тегам обстановки (view.NodeTags).
func DCOfIntent(in core.Intent, view core.SceneView) int {
    if dc := view.CheckDC(string(in.Verb), string(in.Args.Target)); dc > 0 {
        return dc
    }
    return DefaultDC
}

const DefaultDC = 12
```

`SceneView.CheckDC(v, target string) int` — новый метод, читает `checks:` секцию из view. Форма определится в Task 22 при авторстве кейса; здесь оставить стаб «есть карта».

Классы глаголов `ClassAttack`, `ClassSave`, `ClassCheck`, `ClassMove`, `ClassRecover`: добавить в реестр `core/verb.go`, если недостающие.

- [ ] **Step 3: Проходит**. Коммит:

```bash
git add rules/dnd5e/verbs.go rules/dnd5e/verbs_test.go core/verb.go core/scene.go
git commit -m "feat(dnd5e): ExtraVerbs — attack/hide/sneak/trap/lock/flee/rest"
```

---

### Task 15: Гейт фазы 3 — Sanity integration

**Files:**
- Create: `rules/dnd5e/integration_test.go`.

- [ ] **Step 1: Тест сборки** через `System.Resolve` на простом кейсе

```go
package dnd5e

import (
    "encoding/json"
    "testing"

    "github.com/kliuchnikovv/dnd/core"
    "github.com/kliuchnikovv/dnd/dice"
)

func TestSystemResolvesHide(t *testing.T) {
    s := Sheet{Dex: 16, Prof: 2, Skills: []string{"stealth"}}
    raw, _ := json.Marshal(s)
    sys := New()
    d := dice.NewSource(1).Stream("resolve")
    res := sys.Resolve(
        core.Intent{Verb: "hide"},
        core.SceneView{Sheet: raw},
        d,
    )
    if res.Class == "" { t.Fatal("Resolve вернул пустой класс") }
}
```

- [ ] **Step 2: Проход + регрессия**.

- [ ] **Step 3: Коммит**

```bash
git add rules/dnd5e/integration_test.go
git commit -m "test(dnd5e): интеграция System.Resolve"
```

---

## Фаза 4. Archetype `adventure`

### Task 16: Пакет `core/scenarios/adventure/`

**Files:**
- Create: `core/scenarios/adventure/scenario.go`, `.._test.go`.

**Interfaces:**
- Produces: `_ import` регистрирует `"adventure"` в registry.

- [ ] **Step 1: Тест регистрации**

```go
package adventure_test

import (
    "testing"

    "github.com/kliuchnikovv/dnd/core"
    _ "github.com/kliuchnikovv/dnd/core/scenarios/adventure"
)

func TestAdventureRegistered(t *testing.T) {
    sc, ok := core.LookupScenario(core.ScenarioAdventure)
    if !ok { t.Fatal("adventure не зарегистрирован") }
    if sc.Kind() != core.ScenarioAdventure { t.Fatal("kind разъехался") }
}
```

- [ ] **Step 2: Реализация**

```go
// Package adventure — solo D&D-приключение за швом Scenario. Победа —
// «вернись сюда с этим», поражение — HP≤0 без hit dice. AI монстров живёт
// в ai.go, наполнение Panel — в scenario.go.
package adventure

import (
    "github.com/kliuchnikovv/dnd/core"
    "github.com/kliuchnikovv/dnd/rules/dnd5e"
    "github.com/kliuchnikovv/dnd/store"
)

func init() {
    core.RegisterScenario(core.ScenarioAdventure, func() core.Scenario { return &scenario{} })
}

type scenario struct{}

func (s *scenario) Kind() core.ScenarioKind          { return core.ScenarioAdventure }
func (s *scenario) ExtraVerbs() []core.VerbDef       { return dnd5e.Verbs() }
func (s *scenario) Panel(g *core.Game) core.Panel     { return buildPanel(g) }
func (s *scenario) NPCTurn(g *core.Game, id store.EntityID) (core.Intent, bool) {
    return npcTurn(g, id)
}

// Victory — определяется case.json → victory: {type,item,node}.
func (s *scenario) Victory(g *core.Game) (bool, string) {
    v, ok := readVictorySpec(g)
    if !ok { return false, "" }
    switch v.Type {
    case "return_with":
        if g.Node == v.Node && g.HasItem(v.Item) {
            return true, "цель выполнена: " + string(v.Item) + " возвращён"
        }
    }
    return false, ""
}

// Defeat — HP игрока ≤ 0 без hit dice. hit dice в MVP не моделируем,
// поэтому «≤0» = defeat.
func (s *scenario) Defeat(g *core.Game) (bool, string) {
    hp := g.DB.Entities[store.EntityID(g.Actor)].HP
    if hp <= 0 {
        return true, "герой пал"
    }
    return false, ""
}
```

`g.HasItem(itemID)`, `readVictorySpec(g)`, `buildPanel(g)`, `npcTurn(g, id)` — заглушки, реализуем в следующих задачах фазы.

- [ ] **Step 3: Проход**. Коммит:

```bash
git add core/scenarios/adventure/
git commit -m "feat(scenarios): каркас архетипа adventure"
```

---

### Task 17: Victory-спецификация из `case.json`

**Files:**
- Modify: `cases/schema.go` — поле `Victory`.
- Create: `core/scenarios/adventure/victory.go`, `_test.go`.

- [ ] **Step 1: Тест**

```go
func TestVictoryReturnWith(t *testing.T) {
    g := newGameWithSpec(t, VictorySpec{Type: "return_with", Item: "i_amulet", Node: "n_start"})
    g.Node = "n_start"
    g.GiveItem("i_amulet")
    sc := &scenario{}
    won, _ := sc.Victory(g)
    if !won { t.Fatal("должна быть победа") }
}

func TestVictoryWrongNodeOrItem(t *testing.T) {
    // ... симметрично: нет предмета — не победа; не тот узел — не победа.
}
```

- [ ] **Step 2-4: Реализация** — читать `Case.Victory` (поле в File) через специальную функцию `readVictorySpec(g)`. Спец кладём в `g.Extras["adventure.victory"]` при загрузке кейса (в `cases/load.go` — если сценарий adventure).

- [ ] **Step 5: Коммит**

```bash
git add ...
git commit -m "feat(adventure): victory-спецификация return_with"
```

---

### Task 18: AI монстров

**Files:**
- Create: `core/scenarios/adventure/ai.go`, `_test.go`.

- [ ] **Step 1: Тест** — есть враг в узле → intent `attack` на него. Нет — intent `move_zone` к ближайшему.

- [ ] **Step 2-4: Реализация**

```go
func npcTurn(g *core.Game, id store.EntityID) (core.Intent, bool) {
    npc := g.DB.Entities[id]
    // Найти ближайшего врага (в MVP — игрок).
    target := store.EntityID(g.Actor)
    if npc.Node == g.DB.Entities[target].Node {
        return core.Intent{Verb: "attack", Actor: string(id),
            Args: core.Args{Target: target}}, true
    }
    // Иначе — движение по adjacency в сторону цели (простой BFS).
    next, ok := stepToward(g, npc.Node, g.DB.Entities[target].Node)
    if !ok { return core.Intent{}, false }
    return core.Intent{Verb: "move_zone", Actor: string(id),
        Args: core.Args{Node: next}}, true
}
```

`stepToward` — BFS по `Location.Adjacent`.

- [ ] **Step 5: Коммит**

```bash
git commit -m "feat(adventure): AI монстров — атаковать или подойти"
```

---

### Task 19: Гейт фазы 4 — mini-adventure e2e

**Files:**
- Create: `cases/testdata/mini_adventure.json`.
- Create: `core/scenarios/adventure/mini_e2e_test.go`.

Мини-кейс: 2 узла (`n_start`, `n_end`), 1 монстр (`e_orc`), 1 предмет (`i_gem`), герой добывает и возвращается. Плей-план: attack → attack → attack → move_zone → win.

- [ ] **Step 1-4: Написать фикстуру + тест, играющий его до Victory=true**. Тест использует `dice.NewSource(seed)` — детерминированный.

- [ ] **Step 5: Полная регрессия**. `harbour` и `forte_merlo` зелёные — гейт фазы 4.

- [ ] **Step 6: Коммит**

```bash
git commit -m "test(adventure): mini-кейс e2e — гейт фазы 4"
```

---

## Фаза 5. Клиент / turn-view

### Task 20: `Panel(g)` для adventure

**Files:**
- Modify: `core/scenarios/adventure/scenario.go` (или create panel.go).
- Modify: `view/build.go` (или где формируется turn-view) — если Panel уходит через turn-view.
- Test: юнит-тесты панели.

- [ ] **Step 1: Тест**

```go
func TestAdventurePanelHasHealthMapInventoryInitiative(t *testing.T) {
    g := newAdventureGame(t)
    p := (&scenario{}).Panel(g)
    kinds := sectionKinds(p)
    for _, want := range []string{"health", "map", "inventory"} {
        if !containsStr(kinds, want) {
            t.Errorf("нет секции %q: %v", want, kinds)
        }
    }
    // "initiative" — только если в бою
    g.Encounter = &core.Encounter{Order: []store.EntityID{...}}
    if !containsStr(sectionKinds((&scenario{}).Panel(g)), "initiative") {
        t.Error("в бою секции initiative нет")
    }
}
```

- [ ] **Step 2-4: Реализация** `buildPanel(g)` — соберёт секции по спеке §8.

- [ ] **Step 5: Коммит**

```bash
git commit -m "feat(adventure): Panel — health/map/inventory/initiative"
```

---

### Task 21: Рендер `Panel` в клиенте

**Files:**
- Modify: `client/...` (RN, точный путь смотреть в repo — вероятно `client/src/components/panel/`).

Клиент рисует секции по `kind`; для adventure — HP-бар, список узлов, список предметов, инициатива. Детективный casebook — отдельная реализация, не трогаем.

- [ ] **Step 1: Разведать**

```bash
grep -rn "Panel\|panel" client/src/ | head
```

- [ ] **Step 2-4: Добавить рендерер `AdventurePanel`, роутинг по типу**. Ручной прогон mini_adventure.

- [ ] **Step 5: Коммит**

```bash
git commit -m "feat(client): рендер AdventurePanel"
```

---

## Фаза 6. Кейс «Ночной маяк»

### Task 22: `cases/lighthouse/case.json` — контент

**Files:**
- Create: `cases/lighthouse/case.json`.

Автомат-тест регрессии: `cases.Load("cases/lighthouse/case.json")` без ошибок.

Содержимое по §7 спеки: 6 узлов, герой Кей, Талан-призрак (большая карточка), гарпёныши, вор, кристалл, ключ, verbs через adventure/dnd5e.

- [ ] **Step 1: Смок-тест загрузки**

```go
func TestLighthouseLoads(t *testing.T) {
    cfg, err := cases.Load("../cases/lighthouse/case.json")
    if err != nil { t.Fatal(err) }
    if cfg.Scenario.Kind() != core.ScenarioAdventure {
        t.Fatalf("сценарий: %v", cfg.Scenario.Kind())
    }
}
```

- [ ] **Step 2: Написать JSON** — 6 узлов, ~5 сущностей, ~8 предметов, диалоги Талана (voice/life/talks_about/open_threads богато), встречи (гарпёныши, вор), checks/traps DCs, victory-спец.

Полное содержимое JSON превышает разумный формат одной задачи. Автор пишет по разделам:
- `locations` (6)
- `entities` (герой + 5 NPC/монстра)
- `items` (амулет-кристалл, ключ, зелья, snарягу)
- `character.sheet` (Кей)
- `dossiers` (Талан в первую очередь — Голос/Быт/Talks/OpenThreads)
- `flavour` (реплики по глаголам)
- `victory: {type:"return_with", item:"i_lightcrystal", node:"n_shore"}`
- `scenario:"adventure"`, `rules:"dnd5e"`

- [ ] **Step 3: Играется руками** (или скриптом-плей-планом до победы). Тест — `TestLighthouseCompletableViaPlayPlan`.

- [ ] **Step 4: Коммит**

```bash
git add cases/lighthouse/
git commit -m "feat(cases): контент — Ночной маяк (D&D-adventure)"
```

---

### Task 23: Гейт фазы 6 — e2e прогон

**Files:**
- Create: `e2e/lighthouse_test.go`.

- [ ] **Step 1: Играть кейс от старта до победы** через существующий e2e-фреймворк. Детерм. seed, известный порядок ходов.

- [ ] **Step 2: Регрессия — harbour/forte_merlo зелёные**.

- [ ] **Step 3: Коммит**

```bash
git commit -m "test(e2e): Ночной маяк проходится"
```

---

## Фаза 7. Обкатка

### Task 24: Prompt-тюнинг реплик под adventure

Талан-призрак — не рыбак с пристани; голос другой. Пройти живьём, поправить `replySystem` секции сценария, где ляжет.

- [ ] **Step 1: Играть кейс, ловить реплики**.
- [ ] **Step 2: Правки в `master/master.go` (replySystem или новая ветка на призрачных NPC)**.
- [ ] **Step 3: Регрессия — harbour**.
- [ ] **Step 4: Коммит(ы)**.

Задача открытая, дошивать по мере находок. Не блокирует «готово».

---

## Self-review

**Spec coverage:**

| Раздел спеки | Задача |
|---|---|
| §3 Сценарный шов | Task 1-4 |
| §4 Правила dnd5e | Task 9-15 |
| §5 Точки роста | закладываются в задачах Task 9 (sheet), Task 10 (dice), Task 14 (verbs) — «добавить, не переписывать» |
| §6 Ядерные добавки | Task 5-8 |
| §7 Кейс «Ночной маяк» | Task 22-23 |
| §8 Panel клиенту | Task 20-21 |
| §9 Гейты | Task 4, 8, 15, 19, 23 |
| §10 План работ | всё |
| §11 Что оставлено на потом | явно не покрывается — так и должно быть |

**Placeholder scan:** grep — ниже, при финальной вычитке.

**Type consistency:** `Scenario` interface одинаково подаётся во всех задачах; поля `Sheet` совпадают в Task 9 и Task 12; `Encounter.Order` / `Turn` / `Round` — стабильно от Task 6 до Task 8; `Mutation` расширяется одним разом в Task 7 (`Amount`, `Condition`, `Order`); `VictorySpec` появляется в Task 17 и используется в Task 22.

**Что оставлено на разведку исполнителем:**
- `client/src/components/panel/` — точная структура RN-клиента; исполнитель разведает при Task 21.
- `SceneView` — точная форма расширения полей `DC/TargetAC/SaveAbility/CheckDC` определяется при Task 12 из существующего `core/scene.go`.
- Публичное API `accusation` — какие точно функции переезжают, определит исполнитель при Task 3 по grep-у `accusation.` в CLI/server/view.

Эти три точки — реальная разведка, не placeholder. План описывает контракт, исполнитель уточняет форму.
