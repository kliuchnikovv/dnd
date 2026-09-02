# Character-owned ruleset flow. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ruleset становится свойством героя, не дела. Игрок создаёт героя в правилах (Порог | D&D 5e), при старте сессии выбирает совместимое дело. Авторский `character:` из case.json уходит совсем.

**Architecture:**

- **Сервер**: character-store поверх существующего `store.DB` — героев хранит пользователь, не кейс. `GET /cases` возвращает список загруженных кейсов с их `{id, name, rules, scenario, blurb}`. Session-start принимает `{case_id, character_id}` и проверяет `case.rules == character.ruleset`.
- **Клиент**: `CharacterCreateScreen` — двухшаговый: сначала ruleset, потом архетип. Character сохраняется в `state/characters` (zustand + async-storage) с полем `ruleset`. `SessionSelectScreen` — сначала выбор персонажа, потом список дел, отфильтрованный по ruleset'у.
- **Данные**: `character:` секция удаляется из harbour/lighthouse/forte_merlo и mini_adventure. Тесты, ждавшие авторского character, перестраиваются: строят character программно и подсовывают в `NewGame`.

**Tech Stack:** Go 1.22+, React Native (Expo SDK 57), zustand.

**Spec:** нет отдельного документа — дизайн зафиксирован диалогом в предыдущей сессии; резюме — в этом header'е.

## Global Constraints

- **Регрессия каждый шаг:** `go test ./...` и `npm test --prefix client` (если возможно) остаются зелёными.
- **Обратной совместимости с case.json нет:** плановое ломающее изменение. Все три существующих кейса (`harbour`, `forte_merlo`, `lighthouse`) правятся вместе с сервером; авторский `character:` уходит из JSON целиком, а не через deprecation flag.
- **Комментарии в стиле репозитория:** русские, по тону как `core/game.go`, `server/runtime.go`.
- **Commit-conventions:** `feat:`/`fix:`/`refactor:`/`test:`/`docs:` префиксы, каждый коммит — из тела одной задачи плана.
- **Clean-room рефакторинг не делаем:** правим только то, что план называет.

---

## Обзор фаз

| Фаза | Тема | Задачи |
|---|---|---|
| 1 | Character-store и schema на сервере | 1-3 |
| 2 | `GET /cases` и session-start контракт | 4-5 |
| 3 | Миграция кейсов (снос `character:` из JSON) | 6 |
| 4 | Клиент: character-state и D&D мок-архетипы | 7-8 |
| 5 | Клиент: двухшаговый create + case-picker | 9-10 |
| 6 | E2E: играем оба режима, гейт | 11 |

---

## Фаза 1. Character-store и schema на сервере

### Task 1: `store.Character` расширяется полем `Ruleset`

**Files:**
- Modify: `store/rows.go` — добавить поле `Ruleset string` в `type Character`.
- Modify: `cases/schema.go` — синхронизировать секцию character при чтении JSON (пока с fallback на "threshold").
- Create: `store/character_ruleset_test.go` — тест на marshal/unmarshal с новым полем.

**Interfaces:**
- Produces: `store.Character.Ruleset string` — валидные значения `"threshold"`, `"dnd5e"`, пустая строка читается как `"threshold"` (backwards compat).

- [ ] **Step 1: Тест**

```go
// store/character_ruleset_test.go
package store

import (
    "encoding/json"
    "testing"
)

func TestCharacterJSONCarriesRuleset(t *testing.T) {
    raw := `{"id":"chr_kay","ruleset":"dnd5e","sheet":{}}`
    var c Character
    if err := json.Unmarshal([]byte(raw), &c); err != nil { t.Fatal(err) }
    if c.Ruleset != "dnd5e" { t.Fatalf("ruleset: %q", c.Ruleset) }
}

func TestCharacterWithoutRulesetIsEmpty(t *testing.T) {
    raw := `{"id":"chr_x","sheet":{}}`
    var c Character
    if err := json.Unmarshal([]byte(raw), &c); err != nil { t.Fatal(err) }
    if c.Ruleset != "" {
        t.Fatalf("должно быть пусто, получено: %q", c.Ruleset)
    }
}
```

- [ ] **Step 2: Упадёт** — поля нет.

- [ ] **Step 3: Реализация**

В `store/rows.go`, в `type Character struct`:

```go
    // Ruleset — правила, в которых создан герой ("threshold" | "dnd5e").
    // Пустая строка при загрузке читается как "threshold" — обратная
    // совместимость с прогонами до character-owned ruleset.
    Ruleset string `json:"ruleset,omitempty"`
```

- [ ] **Step 4: Тесты проходят.**

- [ ] **Step 5: Регрессия** `go test ./...` — зелёно.

- [ ] **Step 6: Коммит**

```
feat(store): Character.Ruleset — правила у героя, не у дела
```

---

### Task 2: `Ruleset` регистр в `core`

**Files:**
- Create: `core/ruleset.go` — константы + registry по имени (аналог `Scenario`).
- Modify: `rules/threshold/resolve.go` — `init()` регистрирует `"threshold"`.
- Modify: `rules/dnd5e/resolve.go` — `init()` регистрирует `"dnd5e"`.
- Create: `core/ruleset_test.go`.

**Interfaces:**
- Produces:
  ```go
  type RulesetKind string
  const RulesetThreshold RulesetKind = "threshold"
  const RulesetDND5e    RulesetKind = "dnd5e"

  func RegisterRuleset(k RulesetKind, factory func() RuleSystem)
  func LookupRuleset(k RulesetKind) (RuleSystem, bool)
  ```

- [ ] **Step 1: Тест**

```go
// core/ruleset_test.go
package core

import "testing"

func TestRulesetRegistryUnknownIsFalse(t *testing.T) {
    if _, ok := LookupRuleset("no-such"); ok {
        t.Fatal("registry соврал про незнакомый ruleset")
    }
}

func TestRulesetRegistryReturnsRegistered(t *testing.T) {
    stub := stubRules{}
    RegisterRuleset("test-ruleset", func() RuleSystem { return stub })
    got, ok := LookupRuleset("test-ruleset")
    if !ok || got == nil { t.Fatal("не вернул зарегистрированный") }
}

type stubRules struct{}
func (stubRules) Resolve(Intent, SceneView, Dice) Resolution { return Resolution{} }
```

- [ ] **Step 2: Упадёт.**

- [ ] **Step 3: Реализация**

```go
// core/ruleset.go
package core

type RulesetKind string

const (
    RulesetThreshold RulesetKind = "threshold"
    RulesetDND5e     RulesetKind = "dnd5e"
)

var rulesetRegistry = map[RulesetKind]func() RuleSystem{}

func RegisterRuleset(k RulesetKind, factory func() RuleSystem) {
    rulesetRegistry[k] = factory
}

func LookupRuleset(k RulesetKind) (RuleSystem, bool) {
    f, ok := rulesetRegistry[k]
    if !ok {
        return nil, false
    }
    return f(), true
}
```

В `rules/threshold/resolve.go` — добавить `init()`:

```go
func init() {
    core.RegisterRuleset(core.RulesetThreshold, func() core.RuleSystem { return New() })
}
```

В `rules/dnd5e/resolve.go` — аналогично для `RulesetDND5e`.

Также добавить blank-imports в `cmd/dnd/main.go` и `cmd/server/main.go`:

```go
_ "github.com/kliuchnikovv/dnd/rules/threshold"
_ "github.com/kliuchnikovv/dnd/rules/dnd5e"
```

- [ ] **Step 4: Тесты + регрессия.**

- [ ] **Step 5: Коммит**

```
feat(core): реестр Ruleset — threshold/dnd5e регистрируются самостоятельно
```

---

### Task 3: Character-хранилище в `store.DB`

**Files:**
- Modify: `store/db.go` — гарантировать что `DB.Characters` работает как store игроков; добавить методы `SaveCharacter`, `Characters()`, `Character(id)`.
- Create: `store/characters_test.go`.

**Interfaces:**
- Produces:
  ```go
  func (db *DB) SaveCharacter(c *Character)
  func (db *DB) Characters() []*Character  // стабильно отсортировано по ID
  func (db *DB) CharacterByID(id CharacterID) (*Character, bool)
  ```

- [ ] **Step 1: Прочитать `store/db.go`** — понять текущую форму `DB.Characters` (уже есть `map[CharacterID]*Character`).

- [ ] **Step 2: Тесты**

```go
package store

import "testing"

func TestSaveCharacterAndList(t *testing.T) {
    db := NewDB()
    db.SaveCharacter(&Character{ID: "chr_a", Ruleset: "threshold"})
    db.SaveCharacter(&Character{ID: "chr_b", Ruleset: "dnd5e"})
    got := db.Characters()
    if len(got) != 2 { t.Fatalf("Characters len=%d", len(got)) }
    if got[0].ID != "chr_a" { t.Fatalf("order: %v", got) }
}

func TestCharacterByIDMissing(t *testing.T) {
    db := NewDB()
    if _, ok := db.CharacterByID("no"); ok {
        t.Fatal("несуществующий персонаж не должен возвращаться")
    }
}
```

- [ ] **Step 3: Реализация** — добавить методы над существующей мапой `Characters`. Сортировка стабильная по ID.

- [ ] **Step 4: Тесты + регрессия.**

- [ ] **Step 5: Коммит**

```
feat(store): методы Character store (Save/Get/List)
```

---

## Фаза 2. `GET /cases` и session-start контракт

### Task 4: `GET /cases` эндпоинт

**Files:**
- Modify: `server/runtime.go` (или где HTTP handlers) — добавить handler `/cases`.
- Create: `server/cases_handler_test.go`.
- Modify: `cmd/server/main.go` — загрузить все кейсы при старте, положить в новый `CaseCatalog`.
- Create: `server/catalog.go` — `CaseCatalog` тип.

**Interfaces:**
- Produces:
  ```go
  GET /cases  →  200 [{"id":"c_harbour","name":"Пристань","rules":"threshold","scenario":"deduction","blurb":"…"}, …]
  ```

- [ ] **Step 1: Найти существующий HTTP-роутер** — grep `mux.HandleFunc\|http.Handle`. Понять паттерн для добавления нового эндпоинта.

- [ ] **Step 2: Тест**

```go
func TestGetCasesReturnsCatalog(t *testing.T) {
    cat := &CaseCatalog{
        entries: []CaseSummary{
            {ID: "c_harbour", Name: "Пристань", Rules: "threshold", Scenario: "deduction"},
            {ID: "c_lighthouse", Name: "Ночной маяк", Rules: "dnd5e", Scenario: "adventure"},
        },
    }
    req := httptest.NewRequest("GET", "/cases", nil)
    rec := httptest.NewRecorder()
    cat.HandleList(rec, req)
    if rec.Code != 200 { t.Fatalf("code: %d", rec.Code) }
    var got []CaseSummary
    if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil { t.Fatal(err) }
    if len(got) != 2 || got[0].ID != "c_harbour" { t.Fatalf("got: %+v", got) }
}
```

- [ ] **Step 3: Реализация** `CaseCatalog`, `CaseSummary`, `HandleList`. Регистрация в маршрутизаторе.

- [ ] **Step 4: `cmd/server/main.go`** — при старте загрузить все кейсы из `cases/`-директории через `filepath.Walk`, распарсить, собрать catalog. Передать в server-конструктор.

Blurb — сейчас `case.json` не несёт этого поля. Первый заход: возвращаем первое предложение `briefing` (обрезка до 200 символов). Не требует правки схемы.

- [ ] **Step 5: Regression** `go test ./...` — зелёно.

- [ ] **Step 6: Коммит**

```
feat(server): GET /cases — каталог загруженных дел
```

---

### Task 5: Session-start принимает `{case_id, character_id}`

**Files:**
- Modify: `server/runtime.go` (или соответствующий handler) — расширить контракт session-start.
- Modify: `cases/load.go` — вынести из `Parse` требование секции `character:`, теперь она **необязательна** (переходный шаг; в Task 6 удалим совсем).
- Create: тесты на mismatch ruleset — `server/session_start_test.go`.

**Interfaces:**
- Produces:
  ```go
  POST /sessions  { "case_id": "c_lighthouse", "character_id": "chr_kay" }
  → 200 { "chat_id": "..." }
  → 400 если ruleset несовместим или character не найден.
  ```

- [ ] **Step 1: Разведать текущий session-start контракт** — grep `POST /sessions|createSession|/sessions"`.

- [ ] **Step 2: Тест** на happy path и на mismatch.

- [ ] **Step 3: Реализация** — принимать `character_id`, доставать `Character` из глобального character-store (пока — прямо в CaseCatalog или отдельный in-memory store; персистентность — отдельная задача, вне этого плана; пока хранение в памяти). Проверить `character.Ruleset == case.Rules`. Использовать `core.LookupRuleset(character.Ruleset)` для получения `RuleSystem`.

**Хранение character**: MVP — in-memory character-store, живущий рядом с CaseCatalog. Персистентность приходит позже (когда добавим login-flow — вне scope этого плана).

- [ ] **Step 4: Тесты + регрессия.**

- [ ] **Step 5: Коммит**

```
feat(server): session-start принимает character_id, проверяет совместимость ruleset
```

---

## Фаза 3. Миграция кейсов

### Task 6: Снос `character:` из case.json

**Files:**
- Modify: `cases/harbour/case.json` — удалить секцию `character:`.
- Modify: `cases/forte_merlo/case.json` — удалить секцию `character:` (если есть).
- Modify: `cases/lighthouse/case.json` — удалить секцию `character:`.
- Modify: `cases/testdata/mini_adventure.json` — удалить секцию `character:`.
- Modify: `cases/schema.go` — удалить поле `Character` из `type File`.
- Modify: `cases/load.go` — убрать код, кладущий character в DB.
- Modify: тесты, полагавшиеся на авторского character — исправить, строя character программно.

**Interfaces:** ломает публичный контракт `cases.File` — уносит поле `Character`. Все потребители `cfg.Actor` (актёр по-умолчанию) остаются валидными: `Actor` — отдельное поле в File.

- [ ] **Step 1: Grep-разведка** — где `f.Character` читается / используется. `grep -rn "Character.Sheet\|Character.Grit\|Character.Harm\|character.id" cases/ core/`.

- [ ] **Step 2: Убрать поле** из `cases/schema.go`, `cases/load.go`. Всё, что читало character из File — переписать: либо character-подгружается извне (сервер передаёт), либо в тестах строится программно.

- [ ] **Step 3: Тесты кейсов** (`cases/harbour/`, `cases/forte_merlo/`, `cases/lighthouse/`) — исправить: не ждать `cfg.DB.Characters[...]` из case.json, а создавать character вручную перед `NewGame`.

- [ ] **Step 4: E2E-тесты** (`e2e/*_test.go`, `core/scenarios/adventure/mini_e2e_test.go`) — подстроить: character создаётся тестом и вручную кладётся в `db.SaveCharacter(...)`.

- [ ] **Step 5: `go test ./...` — зелёно.**

- [ ] **Step 6: Коммит**

```
refactor(cases): character вне case.json — ruleset у героя, не у дела
```

---

## Фаза 4. Клиент: character-state и D&D мок-архетипы

### Task 7: Character-store на клиенте (zustand + async-storage)

**Files:**
- Create: `client/src/state/characters.ts` — zustand store с `characters: Character[]`, `addCharacter`, `getById`, `hydrate` (загрузка из async-storage).
- Create: `client/src/state/__tests__/characters.test.ts`.

**Interfaces:**
- Produces:
  ```ts
  type Ruleset = 'threshold' | 'dnd5e';
  type CharacterRecord = { id: string; name: string; ruleset: Ruleset; archetypeId: string };
  useCharacters(): { characters, add, getById, hydrate };
  ```

- [ ] **Step 1: Изучить существующий state паттерн** — `client/src/state/` — какие zustand-стори уже есть, как хендлится persistence.

- [ ] **Step 2: Тест** на add + rehydrate.

- [ ] **Step 3: Реализация.** Использовать `@react-native-async-storage/async-storage` (уже в deps). ID генерировать через `expo-crypto`.

- [ ] **Step 4: Тесты + `npm test`.**

- [ ] **Step 5: Коммит**

```
feat(client): state — character store с persistence
```

---

### Task 8: D&D мок-архетипы

**Files:**
- Modify: `client/src/mocks/app.ts` — добавить `dnd5eCreation: RulesetCreation` с 4 архетипами: файтер / рог / рейнджер / клерик.

**Interfaces:**
- Produces: `dnd5eCreation: RulesetCreation`, `rulesets: [thresholdCreation, dnd5eCreation]`.

- [ ] **Step 1: Прочитать `thresholdCreation`** — понять форму `Archetype`.

- [ ] **Step 2: Дописать `dnd5eCreation`** с 4 карточками. Статы (Str/Dex/…) и tags для каждого. Пример:

```ts
export const dnd5eCreation: RulesetCreation = {
  id: 'dnd5e', name: 'D&D 5e',
  archetypes: [
    { id: 'fighter', name: 'Файтер', blurb: '…', stats: [{ label: 'Str', value: 14 }, …], tags: ['броня','меч'], token: 'archetype_fighter' },
    { id: 'rogue', name: 'Рог', … },
    { id: 'ranger', name: 'Рейнджер', … },
    { id: 'cleric', name: 'Клерик', … },
  ],
};

export const rulesets = [thresholdCreation, dnd5eCreation];
```

- [ ] **Step 3: Тест** на shape — оба ruleset'а валидны, у каждого архетипа непустое имя.

- [ ] **Step 4: `npm test`.**

- [ ] **Step 5: Коммит**

```
feat(client): моки — D&D 5e ruleset и 4 стартовых архетипа
```

---

## Фаза 5. Клиент: двухшаговый create + case-picker

### Task 9: `CharacterCreateScreen` — двухшаговый ruleset → архетип

**Files:**
- Modify: `client/src/screens/CharacterCreateScreen.tsx` — переписать: сначала шаг «Правила», потом «Архетип», потом «Имя».
- Modify: `client/src/mocks/app.ts` — экспорт `rulesets` (уже сделано в Task 8).

- [ ] **Step 1: Прочитать текущий CharacterCreateScreen.**

- [ ] **Step 2: Тест** (`__tests__/CharacterCreateScreen.test.tsx`) — рендерит две карточки ruleset'ов, клик на «D&D 5e» показывает D&D-архетипы, клик на «Порог» — threshold-архетипы.

- [ ] **Step 3: Реализация.** Local state `pickedRuleset: Ruleset | null`. Пока ruleset не выбран — показать список ruleset'ов. Дальше — экран как раньше, но с архетипами выбранного ruleset'а. На финальном коммите — вызвать `useCharacters().add({...ruleset, ...archetype, name})`.

- [ ] **Step 4: `npm test`.**

- [ ] **Step 5: Коммит**

```
feat(client): CharacterCreateScreen — сначала правила, потом архетип
```

---

### Task 10: `SessionSelectScreen` — case-picker с фильтром

**Files:**
- Modify: `client/src/screens/session/SessionSelectScreen.tsx` — переписать сессию-старт: (1) выбор персонажа из локального store, (2) `GET /cases` → список, (3) фильтр по `character.ruleset === case.rules`, (4) старт сессии с `character_id`.
- Modify: `client/src/net/` — добавить `listCases()` API-клиент.

- [ ] **Step 1: Прочитать текущий SessionSelectScreen.**

- [ ] **Step 2: `net/cases.ts`** — `listCases(token: string): Promise<CaseSummary[]>`.

- [ ] **Step 3: Тест** экрана: рендерит characters из store, при выборе персонажа делает `listCases`, показывает только совместимые.

- [ ] **Step 4: Реализация.** UI: две колонки / два шага — «Кто идёт?» → «Куда?». Кнопка «Начать» активна когда оба выбраны.

- [ ] **Step 5: Убрать `createSession(token, 'harbour')` хардкод** — теперь `createSession(token, caseId, characterId)`.

- [ ] **Step 6: `npm test`.**

- [ ] **Step 7: Коммит**

```
feat(client): SessionSelectScreen — герой + совместимое дело
```

---

## Фаза 6. Гейт

### Task 11: E2E — оба режима играются

**Files:**
- Modify: e2e-тест, играющий harbour от старта до победы, используя threshold-героя, созданного программно.
- Modify: e2e-тест lighthouse аналогично с D&D-героем.
- Verify: `go test ./e2e/...` — зелёный.

- [ ] **Step 1: Найти существующие e2e** — `ls e2e/`. Понять паттерн.

- [ ] **Step 2: Обновить/добавить тесты** — main-line for both cases with programmatic character creation.

- [ ] **Step 3: `go test ./...` — весь зелёный.**

- [ ] **Step 4: Коммит**

```
test(e2e): оба режима играются с character-owned ruleset
```

---

## Self-review

**Spec coverage:** дизайн зафиксирован header'ом плана. Каждый его пункт получил задачу:
- Ruleset как свойство героя → Task 1 (schema), Task 2 (registry), Task 7 (client state).
- `GET /cases` → Task 4.
- Session-start `{case_id, character_id}` → Task 5.
- Снос `character:` из case.json → Task 6.
- Client двухшаговый create → Task 9.
- Client case-picker с фильтром → Task 10.
- E2E оба режима → Task 11.

**Placeholders:** нет TODO/TBD; blurb-обрезка объяснена; хранение character-store в памяти помечено как MVP-выбор.

**Type consistency:** `Ruleset` тип в TS — `'threshold' | 'dnd5e'`; в Go — `core.RulesetKind` (те же значения). `CharacterRecord` в клиенте использует те же поля, что сервер ждёт (`ruleset`, `archetype_id`).

**Оставлено на разведку исполнителю:**
- Точная форма HTTP-роутера — Task 4 Step 1.
- Точный zustand-паттерн этого проекта — Task 7 Step 1.
- Существующий CharacterCreateScreen UI — Task 9 Step 1.
