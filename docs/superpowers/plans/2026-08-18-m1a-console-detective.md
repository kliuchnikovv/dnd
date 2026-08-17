# M1a Console Detective Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Консольный прототип детективной игры на Go — одно рукописное дело, проходимое от первой сцены до верного обвинения, без единого вызова LLM.

**Architecture:** Два независимых слоя. `core` держит граф фактов, знание парти, корроборацию, часы, обвинение и таксономию цены провала; `rules/threshold` держит лист персонажа, модификаторы и бросок. Шов — интерфейс `RuleSystem.Resolve(Intent, SceneView, Dice) Resolution`. Ядро никогда не видит кости, правила никогда не видят граф фактов и `truth`.

**Tech Stack:** Go 1.26, стандартная библиотека. Внешних зависимостей нет ни одной.

**Spec:** `docs/superpowers/specs/2026-08-18-m1a-console-detective-design.md`

## Global Constraints

- Go 1.26, `module github.com/kliuchnikovv/dnd`. **Ни одной внешней зависимости** — `go.sum` остаётся пустым.
- **`core` не импортирует `rules`.** Проверяется тестом через `go list -deps ./core`.
- **`core` не импортирует `math/rand` и не содержит строк `d20`, `grit +`, имён атрибутов.** Проверяется grep-тестом.
- Направление зависимостей: `cli → core → store`, `cli → rules/threshold`, `rules/threshold → core`, `cases → core`. `store` не импортирует ничего из проекта.
- **Ни одного сетевого импорта** (`net/http`, `net`) и ни одного вызова модели во всём репозитории. Проверяется тестом.
- Ни одного флакающего теста: распределение исходов считается точным перебором 20 граней, а не сэмплированием.
- Все тексты, видимые игроку, берутся из `flavour` дела по ключу. Код не конкатенирует прозу.
- Порог — закрытое множество: `Легко 10`, `Норма 14`, `Трудно 18`.
- Классы исхода: маржа ≥ +5 крит, 0…+4 успех, −1…−4 частично, ≤ −5 провал. Нат-20 — ступень вверх, нат-1 — ступень вниз.
- Корроборация: `p = 0.5` на источник, `1 − Π(1 − pᵢ)`, порог действенности `0.8`.
- Ситуативные модификаторы суммируются и клампятся в `±4`.
- Маржа ≤ −10 удваивает список цены провала.

## Уточнения спеки, зафиксированные здесь

Три места, где спека оставляла свободу, и реализация её снимает. Это не отход от спеки, а её доведение до кода:

1. **RNG живёт в отдельном пакете `dice/`, не в `core`.** Спека §4 относит RNG к ядру, но проверка шва §5.5 запрещает `math/rand` в `core/`. Обе цели выполняются так: `core` объявляет интерфейс `Dice`, пакет `dice` реализует именованные стримы.
2. **ID-типы и строки таблиц живут в `store`, `core` импортирует `store`.** Иначе стрелка `core → store` из §4 разворачивается в цикл.
3. **Слоты обвинения оперируют токенами, не `FactID`.** Спека §6.2 типизирует поля `Truth` как `FactID`, но «кто» — это сущность, а «почему» — мотив; факт лишь *открывает* токен. Вводится `type Token string`; связь «токен доступен под собранный факт» задаётся в данных дела таблицей `accusation_tokens`.

---

## Структура файлов

```
go.mod
store/ids.go              типы идентификаторов
store/rows.go             строки таблиц, колонка в колонку со схемой Postgres
store/db.go               DB — набор таблиц + запросы по ним
dice/streams.go           именованные RNG-стримы от seed
core/verbs.go             реестр 30 глаголов, классы, флаг броска
core/intent.go            Intent, Args
core/scene.go             SceneView
core/resolution.go        Outcome, CostKind, Mutation, RollLog, Resolution, Dice, RuleSystem
core/knowledge.go         party_knowledge, банк тем, корроборация
core/clocks.go            тик часов и срабатывание последствия
core/accusation/truth.go  Truth с редакцией, Form, Check
core/turn.go              Apply — пятишаговый ход
core/mutate.go            применение Mutation и исполнение CostKind
rules/threshold/sheet.go  лист персонажа, теги, применимость
rules/threshold/mods.go   порог и ситуативные модификаторы
rules/threshold/resolve.go Resolve, grit, нат-20/нат-1
rules/threshold/cost.go    выбор цены провала по классу глагола
cases/schema.go           JSON-форма дела
cases/load.go             загрузчик
cases/validate.go         валидатор инвариантов
cases/harbour/case.json   рукописное дело
cases/harbour/walkthrough.txt  прохождение до верного обвинения
cli/parse.go              парсер команд
cli/render.go             рендер сцены, facts, state, clocks
cli/repl.go               REPL и скриптовый режим
cmd/dnd/main.go           флаги и точка входа
```

---

### Task 1: Модуль, ID-типы и строки таблиц

**Files:**
- Create: `go.mod`, `store/ids.go`, `store/rows.go`
- Test: `store/rows_test.go`

**Interfaces:**
- Consumes: ничего
- Produces: `store.FactID`, `store.EntityID`, `store.NodeID`, `store.ClockID`, `store.CharacterID`, `store.Token` (все `string`); структуры `store.Fact`, `store.Gate`, `store.FactHolder`, `store.FactUnlock`, `store.Entity`, `store.Location`, `store.Relation`, `store.Knowledge`, `store.Clock`, `store.Character`

- [ ] **Step 1: Создать модуль**

```bash
cd /Users/kliuchnikovv/go/src/github.com/kliuchnikovv/dnd
go mod init github.com/kliuchnikovv/dnd
```

- [ ] **Step 2: Написать падающий тест**

Файл `store/rows_test.go`. Тест проверяет ровно одно: строки таблиц сериализуются в JSON теми именами колонок, которые потом станут колонками Postgres. Это не тавтология — именно здесь ловятся расхождения схемы.

```go
package store

import (
	"encoding/json"
	"testing"
)

func TestFactHolderJSONTags(t *testing.T) {
	h := FactHolder{
		FactID:    "f_ligature",
		HolderID:  "e_body",
		Gate:      Gate{Verbs: []string{"examine"}, Threshold: "normal"},
		Mandatory: true,
		Latent:    false,
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	want := `{"fact_id":"f_ligature","holder_id":"e_body",` +
		`"gate":{"verbs":["examine"],"threshold":"normal","requires":null},` +
		`"mandatory":true,"latent":false}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestKnowledgeCarriesSource(t *testing.T) {
	// Ключ party_knowledge — пара (fact_id, learned_from_entity).
	// Два свидетельства об одном факте — две разные строки.
	a := Knowledge{FactID: "f_shortfall", LearnedFrom: "e_toke", Confidence: 0.5, LearnedAt: 1}
	b := Knowledge{FactID: "f_shortfall", LearnedFrom: "e_sigrid", Confidence: 0.5, LearnedAt: 2}
	if a.Key() == b.Key() {
		t.Fatalf("два источника одного факта дали один ключ: %v", a.Key())
	}
}
```

- [ ] **Step 3: Прогнать тест, убедиться что падает**

Run: `go test ./store/ -run 'TestFactHolderJSONTags|TestKnowledgeCarriesSource' -v`
Expected: FAIL — `undefined: FactHolder`, `undefined: Knowledge`

- [ ] **Step 4: Написать `store/ids.go`**

```go
// Package store держит строки таблиц в памяти. Структура повторяет будущую
// схему Postgres: ссылки идут по идентификаторам, указателей между сущностями
// нет, чтобы переезд в базу был механическим INSERT.
package store

type (
	FactID      string
	EntityID    string
	NodeID      string
	ClockID     string
	CharacterID string
	CaseID      string

	// Token — значение слота обвинения (who / how / when / why).
	// Токен не равен факту: факт лишь открывает токен к использованию.
	Token string
)
```

- [ ] **Step 5: Написать `store/rows.go`**

```go
package store

import "encoding/json"

type FactKind string

const (
	FactConcept    FactKind = "concept"
	FactTransition FactKind = "transition"
)

type EntityKind string

const (
	EntityNPC    EntityKind = "npc"
	EntityRecord EntityKind = "record"
	EntityThing  EntityKind = "thing"
)

type Fact struct {
	ID     FactID   `json:"id"`
	CaseID CaseID   `json:"case_id"`
	Key    string   `json:"key"`
	Kind   FactKind `json:"kind"`
}

// Gate — условие выдачи факта держателем. Threshold — одно из
// "easy" | "normal" | "hard"; интерпретирует его система правил, не ядро.
type Gate struct {
	Verbs     []string `json:"verbs"`
	Threshold string   `json:"threshold"`
	Requires  []FactID `json:"requires"`
}

type FactHolder struct {
	FactID    FactID   `json:"fact_id"`
	HolderID  EntityID `json:"holder_id"`
	Gate      Gate     `json:"gate"`
	Mandatory bool     `json:"mandatory"`
	Latent    bool     `json:"latent"`
}

type FactUnlock struct {
	FactID      FactID `json:"fact_id"`
	UnlocksKind string `json:"unlocks_kind"` // "topic" | "node" | "entity"
	UnlocksID   string `json:"unlocks_id"`
}

type Entity struct {
	ID    EntityID   `json:"id"`
	Name  string     `json:"name"`
	Kind  EntityKind `json:"kind"`
	Voice string     `json:"voice"`
	Node  NodeID     `json:"node"`
}

type Location struct {
	ID       NodeID   `json:"id"`
	Name     string   `json:"name"`
	Adjacent []NodeID `json:"adjacent"`
}

type Relation struct {
	From EntityID `json:"from_entity"`
	To   EntityID `json:"to_entity"`
	Kind string   `json:"kind"`
}

type Knowledge struct {
	FactID      FactID   `json:"fact_id"`
	LearnedFrom EntityID `json:"learned_from_entity"`
	Confidence  float64  `json:"confidence"`
	LearnedAt   int      `json:"learned_at"`
}

// Key — составной ключ таблицы party_knowledge.
func (k Knowledge) Key() [2]string { return [2]string{string(k.FactID), string(k.LearnedFrom)} }

type Clock struct {
	ID         ClockID `json:"id"`
	Name       string  `json:"name"`
	Segments   int     `json:"segments"`
	Filled     int     `json:"filled"`
	TickPolicy string  `json:"tick_policy"`
	// OnFill — ключ флейвора и мутации мира при заполнении. Часы не убивают
	// прогон, они меняют мир.
	OnFill Consequence `json:"on_fill"`
}

type Consequence struct {
	FlavourKey    string     `json:"flavour_key"`
	RemoveHolders []FactID   `json:"remove_holders"`
	HostileTo     []EntityID `json:"hostile_to"`
}

type Character struct {
	ID    CharacterID     `json:"id"`
	Sheet json.RawMessage `json:"sheet"` // непрозрачен для ядра
	Harm  int             `json:"harm"`
	Grit  int             `json:"grit"`
}
```

- [ ] **Step 6: Прогнать тест, убедиться что проходит**

Run: `go test ./store/ -v`
Expected: PASS

- [ ] **Step 7: Коммит**

```bash
git add go.mod store/
git commit -m "feat(store): ID-типы и строки таблиц по схеме Postgres"
```

---

### Task 2: Таблицы `store.DB` и запросы по ним

**Files:**
- Create: `store/db.go`
- Test: `store/db_test.go`

**Interfaces:**
- Consumes: типы из Task 1
- Produces: `store.DB` с полями `Facts map[FactID]Fact`, `Holders map[FactID][]FactHolder`, `Unlocks map[FactID][]FactUnlock`, `Entities map[EntityID]Entity`, `Locations map[NodeID]Location`, `Relations []Relation`, `Clocks map[ClockID]*Clock`, `Knowledge []Knowledge`, `Characters map[CharacterID]*Character`; методы `HoldersOf(FactID) []FactHolder`, `Related(a, b EntityID) bool`, `Adjacent(from, to NodeID) bool`, `EntitiesAt(NodeID) []Entity`

- [ ] **Step 1: Написать падающий тест**

`store/db_test.go`:

```go
package store

import "testing"

func newTestDB() *DB {
	db := NewDB()
	db.Entities["e_ivar"] = Entity{ID: "e_ivar", Name: "Ивар", Kind: EntityNPC, Node: "n_forge"}
	db.Entities["e_toke"] = Entity{ID: "e_toke", Name: "Токе", Kind: EntityNPC, Node: "n_guildhall"}
	db.Entities["e_sigrid"] = Entity{ID: "e_sigrid", Name: "Сигрид", Kind: EntityNPC, Node: "n_forge"}
	db.Relations = append(db.Relations, Relation{From: "e_ivar", To: "e_sigrid", Kind: "married"})
	db.Locations["n_forge"] = Location{ID: "n_forge", Adjacent: []NodeID{"n_quay"}}
	db.Locations["n_quay"] = Location{ID: "n_quay", Adjacent: []NodeID{"n_forge"}}
	return db
}

func TestRelatedIsSymmetric(t *testing.T) {
	db := newTestDB()
	if !db.Related("e_ivar", "e_sigrid") {
		t.Error("ребро не найдено в прямом направлении")
	}
	if !db.Related("e_sigrid", "e_ivar") {
		t.Error("ребро не найдено в обратном направлении")
	}
	if db.Related("e_ivar", "e_toke") {
		t.Error("несвязанные сущности объявлены связанными")
	}
}

func TestEntitiesAtFiltersByNode(t *testing.T) {
	db := newTestDB()
	got := db.EntitiesAt("n_forge")
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 сущности в кузнице, получено %d", len(got))
	}
	// Порядок детерминирован — иначе вывод сцены пляшет от прогона к прогону.
	if got[0].ID != "e_ivar" || got[1].ID != "e_sigrid" {
		t.Errorf("порядок не детерминирован: %v, %v", got[0].ID, got[1].ID)
	}
}

func TestAdjacentRejectsUnlinkedNodes(t *testing.T) {
	db := newTestDB()
	if !db.Adjacent("n_forge", "n_quay") {
		t.Error("смежные узлы объявлены несмежными")
	}
	if db.Adjacent("n_forge", "n_guildhall") {
		t.Error("несмежные узлы объявлены смежными")
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./store/ -run 'TestRelated|TestEntitiesAt|TestAdjacent' -v`
Expected: FAIL — `undefined: NewDB`

- [ ] **Step 3: Написать `store/db.go`**

```go
package store

import "sort"

// DB — набор таблиц в памяти. Ни одного указателя между сущностями:
// связи выражены идентификаторами, как в реляционной схеме.
type DB struct {
	Facts      map[FactID]Fact
	Holders    map[FactID][]FactHolder
	Unlocks    map[FactID][]FactUnlock
	Entities   map[EntityID]Entity
	Locations  map[NodeID]Location
	Relations  []Relation
	Clocks     map[ClockID]*Clock
	Characters map[CharacterID]*Character
	Knowledge  []Knowledge
}

func NewDB() *DB {
	return &DB{
		Facts:      map[FactID]Fact{},
		Holders:    map[FactID][]FactHolder{},
		Unlocks:    map[FactID][]FactUnlock{},
		Entities:   map[EntityID]Entity{},
		Locations:  map[NodeID]Location{},
		Clocks:     map[ClockID]*Clock{},
		Characters: map[CharacterID]*Character{},
	}
}

func (db *DB) HoldersOf(f FactID) []FactHolder { return db.Holders[f] }

// Related сообщает, есть ли ребро между двумя сущностями. Ребро
// ненаправленное: двое, кто общается, за два независимых источника не считаются
// вне зависимости от того, кто в JSON записан слева.
func (db *DB) Related(a, b EntityID) bool {
	for _, r := range db.Relations {
		if (r.From == a && r.To == b) || (r.From == b && r.To == a) {
			return true
		}
	}
	return false
}

func (db *DB) Adjacent(from, to NodeID) bool {
	for _, n := range db.Locations[from].Adjacent {
		if n == to {
			return true
		}
	}
	return false
}

// EntitiesAt возвращает сущности узла в стабильном порядке по ID.
// Итерация по map в Go случайна, а вывод сцены обязан быть воспроизводим.
func (db *DB) EntitiesAt(n NodeID) []Entity {
	var out []Entity
	for _, e := range db.Entities {
		if e.Node == n {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
```

- [ ] **Step 4: Прогнать тест, убедиться что проходит**

Run: `go test ./store/ -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add store/db.go store/db_test.go
git commit -m "feat(store): таблицы DB и запросы по связям и узлам"
```

---

### Task 3: Именованные RNG-стримы

**Files:**
- Create: `dice/streams.go`
- Test: `dice/streams_test.go`

**Interfaces:**
- Consumes: ничего
- Produces: `dice.NewSource(seed int64) *Source`, `(*Source).Stream(name string) *Stream`, `(*Stream).D20() int`, `(*Stream).Roll(n, sides int) int`, `dice.Fixed(values ...int) *FixedDice`

- [ ] **Step 1: Написать падающий тест**

`dice/streams_test.go`. Суть именованных стримов в том, что добавление броска в одну подсистему не сдвигает последовательность в другой — иначе `--seed` перестаёт воспроизводить прогон после любой правки.

```go
package dice

import "testing"

func TestStreamsAreIndependent(t *testing.T) {
	a := NewSource(7)
	b := NewSource(7)

	// В прогоне A из стрима resolve тянут три раза, в прогоне B — сначала
	// два раза из постороннего стрима, потом три из resolve.
	wantA := []int{a.Stream("resolve").D20(), a.Stream("resolve").D20(), a.Stream("resolve").D20()}

	b.Stream("false_lead").D20()
	b.Stream("flavour").D20()
	gotB := []int{b.Stream("resolve").D20(), b.Stream("resolve").D20(), b.Stream("resolve").D20()}

	for i := range wantA {
		if wantA[i] != gotB[i] {
			t.Fatalf("бросок %d разошёлся: %d против %d — стримы не независимы",
				i, wantA[i], gotB[i])
		}
	}
}

func TestSameSeedSameSequence(t *testing.T) {
	x := NewSource(42).Stream("resolve")
	y := NewSource(42).Stream("resolve")
	for i := 0; i < 50; i++ {
		if v, w := x.D20(), y.D20(); v != w {
			t.Fatalf("бросок %d: %d != %d — один seed дал разные последовательности", i, v, w)
		}
	}
}

func TestDifferentSeedsDiverge(t *testing.T) {
	x := NewSource(1).Stream("resolve")
	y := NewSource(2).Stream("resolve")
	same := 0
	for i := 0; i < 50; i++ {
		if x.D20() == y.D20() {
			same++
		}
	}
	if same == 50 {
		t.Fatal("разные seed дали одинаковую последовательность")
	}
}

func TestD20StaysInRange(t *testing.T) {
	s := NewSource(3).Stream("resolve")
	for i := 0; i < 10000; i++ {
		if v := s.D20(); v < 1 || v > 20 {
			t.Fatalf("d20 вернул %d вне диапазона 1..20", v)
		}
	}
}

func TestFixedDiceReplaysValuesThenRepeatsLast(t *testing.T) {
	d := Fixed(20, 1)
	if got := d.D20(); got != 20 {
		t.Errorf("первый бросок: %d, ожидалось 20", got)
	}
	if got := d.D20(); got != 1 {
		t.Errorf("второй бросок: %d, ожидалось 1", got)
	}
	if got := d.D20(); got != 1 {
		t.Errorf("третий бросок: %d, ожидалось повторение последнего (1)", got)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./dice/ -v`
Expected: FAIL — `undefined: NewSource`

- [ ] **Step 3: Написать `dice/streams.go`**

```go
// Package dice реализует именованные RNG-стримы. Ядро объявляет интерфейс
// Dice, но math/rand живёт здесь: core обязан оставаться свободным от костей.
package dice

import (
	"hash/fnv"
	"math/rand"
)

// Source раздаёт независимые стримы от одного seed. Стрим с именем name
// засеян hash(seed, name), поэтому добавление броска в одной подсистеме не
// сдвигает последовательность в другой.
type Source struct {
	seed    int64
	streams map[string]*Stream
}

func NewSource(seed int64) *Source {
	return &Source{seed: seed, streams: map[string]*Stream{}}
}

// Stream возвращает стрим по имени, создавая его при первом обращении.
// Повторный вызов с тем же именем отдаёт тот же стрим, а не новый.
func (s *Source) Stream(name string) *Stream {
	if st, ok := s.streams[name]; ok {
		return st
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	st := &Stream{r: rand.New(rand.NewSource(s.seed ^ int64(h.Sum64())))}
	s.streams[name] = st
	return st
}

type Stream struct{ r *rand.Rand }

func (s *Stream) D20() int { return s.r.Intn(20) + 1 }

func (s *Stream) Roll(n, sides int) int {
	total := 0
	for i := 0; i < n; i++ {
		total += s.r.Intn(sides) + 1
	}
	return total
}

// FixedDice подставляет заданные значения — для тестов, где бросок должен быть
// известен заранее. Исчерпав список, повторяет последнее значение.
type FixedDice struct {
	values []int
	i      int
}

func Fixed(values ...int) *FixedDice { return &FixedDice{values: values} }

func (f *FixedDice) D20() int {
	if len(f.values) == 0 {
		return 10
	}
	v := f.values[f.i]
	if f.i < len(f.values)-1 {
		f.i++
	}
	return v
}

func (f *FixedDice) Roll(n, sides int) int { return f.D20() }
```

- [ ] **Step 4: Прогнать тест, убедиться что проходит**

Run: `go test ./dice/ -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add dice/
git commit -m "feat(dice): именованные RNG-стримы и фиксированные кости для тестов"
```

---

### Task 4: Реестр глаголов

**Files:**
- Create: `core/verbs.go`
- Test: `core/verbs_test.go`

**Interfaces:**
- Consumes: ничего
- Produces: `core.Verb` (string), `core.VerbClass` (string) с константами `ClassNone|ClassInvestigate|ClassReason|ClassSocial|ClassMove|ClassAttack|ClassSupport|ClassResource|ClassSkill`, `core.VerbDef{Verb, Class, Rolls, Hard}`, `core.Verbs map[Verb]VerbDef`, `core.LookupVerb(string) (VerbDef, bool)`, `core.AllVerbs() []VerbDef`

- [ ] **Step 1: Написать падающий тест**

`core/verbs_test.go`. Число 26 из брифа не совпадает с перечислением; источник истины — перечисление, поэтому тест сверяет состав, а не константу.

```go
package core

import "testing"

func TestVerbRegistryCoversEveryListedVerb(t *testing.T) {
	want := []Verb{
		"look", "emote", "say",
		"talk_to", "ask_about", "thank", "threaten_verbally", "theorize",
		"examine", "search", "question", "stake_out", "tail",
		"compare", "cross_reference",
		"strike", "grapple",
		"move_zone", "take_cover", "flee",
		"sneak", "pick", "recall",
		"persuade", "intimidate", "command",
		"aid", "mend",
		"use_ability", "use_item",
	}
	if len(want) != 30 {
		t.Fatalf("список в тесте испорчен: %d глаголов вместо 30", len(want))
	}
	for _, v := range want {
		if _, ok := Verbs[v]; !ok {
			t.Errorf("глагол %q отсутствует в реестре", v)
		}
	}
	if len(Verbs) != len(want) {
		t.Errorf("в реестре %d глаголов, в списке %d — есть лишние", len(Verbs), len(want))
	}
}

func TestEveryVerbHasKnownClass(t *testing.T) {
	known := map[VerbClass]bool{
		ClassNone: true, ClassInvestigate: true, ClassReason: true,
		ClassSocial: true, ClassMove: true, ClassAttack: true,
		ClassSupport: true, ClassResource: true, ClassSkill: true,
	}
	for v, d := range Verbs {
		if !known[d.Class] {
			t.Errorf("глагол %q имеет неизвестный класс %q", v, d.Class)
		}
	}
}

func TestCompareNeverRolls(t *testing.T) {
	// Противоречие между двумя известными фактами — свойство данных, не удача.
	if Verbs["compare"].Rolls {
		t.Error("compare требует броска — это ошибка правил, а не реализации")
	}
	if !Verbs["cross_reference"].Rolls {
		t.Error("cross_reference обязан требовать броска")
	}
}

func TestFlavourAndSoftVerbsNeverRoll(t *testing.T) {
	for _, v := range []Verb{"look", "emote", "say", "talk_to", "ask_about",
		"thank", "threaten_verbally", "theorize"} {
		if Verbs[v].Rolls {
			t.Errorf("глагол %q не должен требовать броска", v)
		}
	}
}

func TestLookupVerbRejectsUnknown(t *testing.T) {
	if _, ok := LookupVerb("hack_the_gibson"); ok {
		t.Error("неизвестный глагол опознан как известный")
	}
	if d, ok := LookupVerb("question"); !ok || d.Class != ClassInvestigate {
		t.Errorf("question опознан неверно: %+v %v", d, ok)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run TestVerb -v`
Expected: FAIL — `undefined: Verbs`

- [ ] **Step 3: Написать `core/verbs.go`**

```go
// Package core держит граф фактов, знание парти, часы, обвинение и таксономию
// цены провала. Ядро не знает ни про d20, ни про атрибуты, ни про grit.
package core

type Verb string

type VerbClass string

const (
	ClassNone        VerbClass = "none"
	ClassInvestigate VerbClass = "investigate"
	ClassReason      VerbClass = "reason"
	ClassSocial      VerbClass = "social"
	ClassMove        VerbClass = "move"
	ClassAttack      VerbClass = "attack"
	ClassSupport     VerbClass = "support"
	ClassResource    VerbClass = "resource"
	ClassSkill       VerbClass = "skill"
)

type VerbDef struct {
	Verb  Verb
	Class VerbClass
	// Rolls — требует ли глагол броска. У use_ability и use_item значение
	// перекрывается флагом requires_roll в данных дела.
	Rolls bool
	Hard  bool
}

var Verbs = map[Verb]VerbDef{
	"look":              {"look", ClassNone, false, false},
	"emote":             {"emote", ClassNone, false, false},
	"say":               {"say", ClassNone, false, false},
	"talk_to":           {"talk_to", ClassSocial, false, false},
	"ask_about":         {"ask_about", ClassSocial, false, false},
	"thank":             {"thank", ClassSocial, false, false},
	"threaten_verbally": {"threaten_verbally", ClassSocial, false, false},
	"theorize":          {"theorize", ClassReason, false, false},
	"examine":           {"examine", ClassInvestigate, true, true},
	"search":            {"search", ClassInvestigate, true, true},
	"question":          {"question", ClassInvestigate, true, true},
	"stake_out":         {"stake_out", ClassInvestigate, true, true},
	"tail":              {"tail", ClassInvestigate, true, true},
	"compare":           {"compare", ClassReason, false, true},
	"cross_reference":   {"cross_reference", ClassReason, true, true},
	"strike":            {"strike", ClassAttack, true, true},
	"grapple":           {"grapple", ClassAttack, true, true},
	"move_zone":         {"move_zone", ClassMove, true, true},
	"take_cover":        {"take_cover", ClassMove, true, true},
	"flee":              {"flee", ClassMove, true, true},
	"sneak":             {"sneak", ClassSkill, true, true},
	"pick":              {"pick", ClassSkill, true, true},
	"recall":            {"recall", ClassSkill, true, true},
	"persuade":          {"persuade", ClassSocial, true, true},
	"intimidate":        {"intimidate", ClassSocial, true, true},
	"command":           {"command", ClassSocial, true, true},
	"aid":               {"aid", ClassSupport, true, true},
	"mend":              {"mend", ClassSupport, true, true},
	"use_ability":       {"use_ability", ClassResource, true, true},
	"use_item":          {"use_item", ClassResource, true, true},
}

func LookupVerb(s string) (VerbDef, bool) {
	d, ok := Verbs[Verb(s)]
	return d, ok
}

// AllVerbs возвращает определения в стабильном порядке — для table-driven
// тестов и для вывода help.
func AllVerbs() []VerbDef {
	out := make([]VerbDef, 0, len(Verbs))
	for _, d := range Verbs {
		out = append(out, d)
	}
	sortVerbDefs(out)
	return out
}

func sortVerbDefs(v []VerbDef) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j].Verb < v[j-1].Verb; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
```

- [ ] **Step 4: Прогнать тест, убедиться что проходит**

Run: `go test ./core/ -run TestVerb -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add core/verbs.go core/verbs_test.go
git commit -m "feat(core): реестр 30 глаголов с классами и флагом броска"
```

---

### Task 5: Типы шва — Intent, SceneView, Resolution, Dice, RuleSystem

**Files:**
- Create: `core/intent.go`, `core/scene.go`, `core/resolution.go`
- Test: `core/resolution_test.go`

**Interfaces:**
- Consumes: `store` (Task 1), `core.Verb` (Task 4)
- Produces: `core.Args`, `core.Intent`, `core.SceneView`, `core.Outcome` с `OutcomeFail|OutcomePartial|OutcomeSuccess|OutcomeCrit` и методами `Up()`, `Down()`, `String()`; `core.CostKind` с восемью константами; `core.MutationKind`, `core.Mutation`; `core.RollTerm`, `core.RollLog`; `core.Resolution`; `core.Dice`; `core.RuleSystem`

- [ ] **Step 1: Написать падающий тест**

`core/resolution_test.go`:

```go
package core

import "testing"

func TestOutcomeStepsUpAndDownWithinBounds(t *testing.T) {
	cases := []struct {
		in       Outcome
		up, down Outcome
	}{
		{OutcomeFail, OutcomePartial, OutcomeFail},       // ниже провала ступени нет
		{OutcomePartial, OutcomeSuccess, OutcomeFail},
		{OutcomeSuccess, OutcomeCrit, OutcomePartial},
		{OutcomeCrit, OutcomeCrit, OutcomeSuccess},       // выше крита ступени нет
	}
	for _, c := range cases {
		if got := c.in.Up(); got != c.up {
			t.Errorf("%v.Up() = %v, ожидалось %v", c.in, got, c.up)
		}
		if got := c.in.Down(); got != c.down {
			t.Errorf("%v.Down() = %v, ожидалось %v", c.in, got, c.down)
		}
	}
}

func TestEveryCostKindHasName(t *testing.T) {
	all := AllCostKinds()
	if len(all) != 8 {
		t.Fatalf("в таксономии %d элементов, ожидалось 8", len(all))
	}
	seen := map[CostKind]bool{}
	for _, c := range all {
		if c == "" {
			t.Error("пустой элемент таксономии")
		}
		if seen[c] {
			t.Errorf("дубликат в таксономии: %q", c)
		}
		seen[c] = true
	}
}

func TestOutcomeStringIsHumanReadable(t *testing.T) {
	want := map[Outcome]string{
		OutcomeFail: "ПРОВАЛ", OutcomePartial: "ЧАСТИЧНО",
		OutcomeSuccess: "УСПЕХ", OutcomeCrit: "КРИТ",
	}
	for o, w := range want {
		if got := o.String(); got != w {
			t.Errorf("%d.String() = %q, ожидалось %q", int(o), got, w)
		}
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run 'TestOutcome|TestEveryCostKind' -v`
Expected: FAIL — `undefined: Outcome`

- [ ] **Step 3: Написать `core/intent.go`**

```go
package core

import "github.com/kliuchnikovv/dnd/store"

// Args — однородные аргументы всех глаголов. Незаполненные поля пусты;
// какие именно нужны, решает валидация конкретного глагола.
type Args struct {
	Target   store.EntityID
	Topic    store.FactID
	Node     store.NodeID
	Facts    []store.FactID
	Item     string
	Ability  string
	TagClaim string
	Text     string
}

type Intent struct {
	Verb  Verb
	Actor store.CharacterID
	Args  Args
}
```

- [ ] **Step 4: Написать `core/scene.go`**

```go
package core

import (
	"encoding/json"

	"github.com/kliuchnikovv/dnd/store"
)

// SceneView — всё, что система правил видит о мире. Здесь намеренно нет графа
// фактов и нет cases.truth: правила физически не могут их прочитать.
type SceneView struct {
	Node       store.NodeID
	NodeTags   []string // dark, crowd, rain, indoors, ...
	Allies     int
	Foes       int
	Undetected bool
	Cover      bool
	ActorTier  int
	TargetTier int
	Tools      []string
	Harm       int // заполненные ячейки ранений
	Grit       int
	// GateThreshold — сложность, заявленная данными дела: "easy"|"normal"|"hard".
	// Пусто, если действие не привязано к держателю факта.
	GateThreshold string
	// Sheet — лист персонажа. Для ядра это байты; разбирает их система правил.
	Sheet json.RawMessage
}

// HasTag сообщает, присутствует ли тег обстановки. Применимость тегов
// персонажа проверяется по спискам, а не по смыслу.
func (v SceneView) HasTag(tag string) bool {
	for _, t := range v.NodeTags {
		if t == tag {
			return true
		}
	}
	return false
}

func (v SceneView) HasTool(tool string) bool {
	for _, t := range v.Tools {
		if t == tool {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Написать `core/resolution.go`**

```go
package core

// Outcome — класс исхода. Порядок значений задаёт ступени: нат-20 поднимает
// на ступень, нат-1 опускает.
type Outcome int

const (
	OutcomeFail Outcome = iota
	OutcomePartial
	OutcomeSuccess
	OutcomeCrit
)

func (o Outcome) Up() Outcome {
	if o == OutcomeCrit {
		return OutcomeCrit
	}
	return o + 1
}

func (o Outcome) Down() Outcome {
	if o == OutcomeFail {
		return OutcomeFail
	}
	return o - 1
}

func (o Outcome) String() string {
	switch o {
	case OutcomeCrit:
		return "КРИТ"
	case OutcomeSuccess:
		return "УСПЕХ"
	case OutcomePartial:
		return "ЧАСТИЧНО"
	default:
		return "ПРОВАЛ"
	}
}

// CostKind — закрытая таксономия цены провала. Система правил выбирает
// элементы отсюда; исполняет их ядро. Ни одна сторона не делает обе вещи.
type CostKind string

const (
	CostTickClock       CostKind = "tick_clock"
	CostFalseLead       CostKind = "false_lead"
	CostDebt            CostKind = "debt"
	CostDispositionDown CostKind = "disposition_down"
	CostPositionWorse   CostKind = "position_worse"
	CostHarmSelf        CostKind = "harm_self"
	CostResourceSpent   CostKind = "resource_spent"
	CostHalfEffect      CostKind = "half_effect"
)

func AllCostKinds() []CostKind {
	return []CostKind{
		CostTickClock, CostFalseLead, CostDebt, CostDispositionDown,
		CostPositionWorse, CostHarmSelf, CostResourceSpent, CostHalfEffect,
	}
}

type MutationKind string

const (
	MutResource    MutationKind = "resource"
	MutClock       MutationKind = "clock"
	MutHarm        MutationKind = "harm"
	MutDisposition MutationKind = "disposition"
	MutPosition    MutationKind = "position"
)

// Mutation обобщена намеренно: «слот 3 уровня» — это {resource, "slot_3", -1},
// а не поле в структуре ядра. Имена ресурсов даёт система правил.
type Mutation struct {
	Kind   MutationKind
	Target string
	Delta  int
}

type RollTerm struct {
	Name  string
	Value int
}

// RollLog — единственный способ, которым значение броска попадает наружу:
// для показа игроку. Ядро им не управляет.
type RollLog struct {
	Die       int
	Terms     []RollTerm
	Total     int
	Threshold int
}

type Resolution struct {
	Class     Outcome
	Margin    int
	Costs     []CostKind
	Mutations []Mutation
	Log       RollLog
}

// Dice объявлен здесь, но реализован в пакете dice: core обязан оставаться
// свободным от math/rand.
type Dice interface {
	D20() int
	Roll(n, sides int) int
}

// RuleSystem — шов. Единственная точка, где ядро обращается к правилам.
type RuleSystem interface {
	Resolve(Intent, SceneView, Dice) Resolution
}
```

- [ ] **Step 6: Прогнать тест, убедиться что проходит**

Run: `go test ./core/ -v`
Expected: PASS

- [ ] **Step 7: Коммит**

```bash
git add core/intent.go core/scene.go core/resolution.go core/resolution_test.go
git commit -m "feat(core): типы шва — Intent, SceneView, Resolution, Dice, RuleSystem"
```

---

### Task 6: Изоляция `truth` и форма обвинения

**Files:**
- Create: `core/accusation/truth.go`
- Test: `core/accusation/truth_test.go`

**Interfaces:**
- Consumes: `store.Token` (Task 1)
- Produces: `accusation.Truth`, `accusation.NewTruth(who, how, when, why store.Token) Truth`, `accusation.Form` со слотами `Who/How/When/Why store.Token`, `(Truth).Check(Form) bool`, `accusation.ErrTruthNotSerializable`

- [ ] **Step 1: Написать падающий тест**

`core/accusation/truth_test.go`. Два разных инварианта: truth не печатается ни при каких обстоятельствах, и частичное совпадение не даёт информации.

```go
package accusation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func testTruth() Truth {
	return NewTruth("toke", "seal_cord", "night_before_tide", "audit_shortfall")
}

func TestTruthNeverPrints(t *testing.T) {
	tr := testTruth()
	for _, s := range []string{
		fmt.Sprint(tr),
		fmt.Sprintf("%v", tr),
		fmt.Sprintf("%s", tr),
		tr.String(),
	} {
		if s != "<redacted>" {
			t.Errorf("truth просочился в вывод: %q", s)
		}
		for _, leak := range []string{"toke", "seal_cord", "night_before_tide", "audit_shortfall"} {
			if strings.Contains(s, leak) {
				t.Errorf("вывод содержит %q", leak)
			}
		}
	}
}

func TestTruthRefusesToMarshal(t *testing.T) {
	if _, err := json.Marshal(testTruth()); err == nil {
		t.Fatal("truth сериализовался в JSON — утечка через любой дамп состояния")
	}
}

func TestCheckAcceptsOnlyAllFourSlots(t *testing.T) {
	tr := testTruth()
	full := Form{Who: "toke", How: "seal_cord", When: "night_before_tide", Why: "audit_shortfall"}
	if !tr.Check(full) {
		t.Fatal("верное обвинение отвергнуто")
	}
	// Три из четырёх — всё ещё неверно.
	three := full
	three.Why = "old_grudge"
	if tr.Check(three) {
		t.Error("обвинение с одной ошибкой принято")
	}
	if tr.Check(Form{}) {
		t.Error("пустое обвинение принято")
	}
}

func TestCheckReturnsOnlyBool(t *testing.T) {
	// Тип возврата — единственный булев: интерфейс не может проговориться,
	// в каком слоте ошибка. Тест фиксирует это на уровне сигнатуры.
	tr := testTruth()
	var _ func(Form) bool = tr.Check
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/accusation/ -v`
Expected: FAIL — `undefined: NewTruth`

- [ ] **Step 3: Написать `core/accusation/truth.go`**

```go
// Package accusation изолирует cases.truth. Правильный ответ не покидает этот
// пакет иначе как одним булевым значением из Check.
package accusation

import (
	"errors"

	"github.com/kliuchnikovv/dnd/store"
)

var ErrTruthNotSerializable = errors.New("accusation: truth не подлежит сериализации")

// Truth хранит правильный ответ. Поля приватны, String редактирован,
// MarshalJSON отказывает — напечатать truth случайно нельзя.
type Truth struct {
	who, how, when, why store.Token
}

func NewTruth(who, how, when, why store.Token) Truth {
	return Truth{who: who, how: how, when: when, why: why}
}

func (Truth) String() string { return "<redacted>" }

func (Truth) MarshalJSON() ([]byte, error) { return nil, ErrTruthNotSerializable }

// Form — заполненная игроком форма обвинения.
type Form struct {
	Who  store.Token `json:"who"`
	How  store.Token `json:"how"`
	When store.Token `json:"when"`
	Why  store.Token `json:"why"`
}

func (f Form) Complete() bool {
	return f.Who != "" && f.How != "" && f.When != "" && f.Why != ""
}

// Check — целиком и символьно. Возврат один булев: игрок не узнаёт, какой
// слот ошибочен, иначе форма брутфорсится по слоту за раз.
func (t Truth) Check(f Form) bool {
	return f.Who == t.who && f.How == t.how && f.When == t.when && f.Why == t.why
}
```

- [ ] **Step 4: Прогнать тест, убедиться что проходит**

Run: `go test ./core/accusation/ -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add core/accusation/
git commit -m "feat(core): Truth с редакцией вывода и целостная проверка обвинения"
```

---

### Task 7: Знание парти, банк тем и корроборация

**Files:**
- Create: `core/knowledge.go`
- Test: `core/knowledge_test.go`

**Interfaces:**
- Consumes: `store.DB` (Task 2)
- Produces: `core.Knowledge` c `NewKnowledge(*store.DB) *Knowledge`, `(*Knowledge).Learn(store.FactID, store.EntityID) bool`, `(*Knowledge).Knows(store.FactID) bool`, `(*Knowledge).Sources(store.FactID) []store.EntityID`, `(*Knowledge).Confidence(store.FactID) float64`, `(*Knowledge).Corroborated(store.FactID) bool`, `(*Knowledge).TopicBank() []store.FactID`; константы `core.SourceConfidence = 0.5`, `core.CorroborationThreshold = 0.8`

- [ ] **Step 1: Написать падающий тест**

`core/knowledge_test.go`:

```go
package core

import (
	"math"
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func knowledgeDB() *store.DB {
	db := store.NewDB()
	db.Facts["f_shortfall"] = store.Fact{ID: "f_shortfall", Key: "shortfall"}
	for _, id := range []store.EntityID{"e_toke", "e_sigrid", "e_bern", "e_nils"} {
		db.Entities[id] = store.Entity{ID: id, Kind: store.EntityNPC}
	}
	// Сигрид и Нильс общаются — за два независимых источника не считаются.
	db.Relations = append(db.Relations, store.Relation{From: "e_sigrid", To: "e_nils", Kind: "aunt"})
	return db
}

func TestTwoSourcesOneFactAreTwoRows(t *testing.T) {
	k := NewKnowledge(knowledgeDB())
	if !k.Learn("f_shortfall", "e_toke") {
		t.Fatal("первый источник не записан")
	}
	if !k.Learn("f_shortfall", "e_bern") {
		t.Fatal("второй источник не записан")
	}
	if got := len(k.Sources("f_shortfall")); got != 2 {
		t.Errorf("источников %d, ожидалось 2", got)
	}
	// Повторное свидетельство того же источника новой строки не даёт.
	if k.Learn("f_shortfall", "e_toke") {
		t.Error("дубликат источника записан как новый")
	}
}

func TestConfidenceCombinesIndependentSources(t *testing.T) {
	k := NewKnowledge(knowledgeDB())
	k.Learn("f_shortfall", "e_toke")
	assertClose(t, k.Confidence("f_shortfall"), 0.5)
	k.Learn("f_shortfall", "e_bern")
	assertClose(t, k.Confidence("f_shortfall"), 0.75)
	k.Learn("f_shortfall", "e_sigrid")
	assertClose(t, k.Confidence("f_shortfall"), 0.875)
	if !k.Corroborated("f_shortfall") {
		t.Error("три независимых источника не дали корроборации")
	}
}

func TestRelatedSourcesDoNotStack(t *testing.T) {
	k := NewKnowledge(knowledgeDB())
	k.Learn("f_shortfall", "e_toke")
	k.Learn("f_shortfall", "e_sigrid")
	k.Learn("f_shortfall", "e_nils") // связан с Сигрид — не засчитывается
	assertClose(t, k.Confidence("f_shortfall"), 0.75)
	if k.Corroborated("f_shortfall") {
		t.Error("связанные источники дали корроборацию")
	}
}

func TestTopicBankHoldsOnlyKnownFacts(t *testing.T) {
	k := NewKnowledge(knowledgeDB())
	if len(k.TopicBank()) != 0 {
		t.Fatal("банк тем непуст на старте — спрашивать не о чем")
	}
	k.Learn("f_shortfall", "e_toke")
	bank := k.TopicBank()
	if len(bank) != 1 || bank[0] != "f_shortfall" {
		t.Errorf("банк тем = %v, ожидалось [f_shortfall]", bank)
	}
}

func assertClose(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("confidence = %v, ожидалось %v", got, want)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run 'TestTwoSources|TestConfidence|TestRelatedSources|TestTopicBank' -v`
Expected: FAIL — `undefined: NewKnowledge`

- [ ] **Step 3: Написать `core/knowledge.go`**

```go
package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

const (
	// SourceConfidence — вклад одного источника.
	SourceConfidence = 0.5
	// CorroborationThreshold — порог действенности. При p=0.5 достигается
	// тремя независимыми источниками: 1-0.5^3 = 0.875.
	CorroborationThreshold = 0.8
)

// Knowledge — таблица party_knowledge и операции над ней.
type Knowledge struct {
	db   *store.DB
	tick int
}

func NewKnowledge(db *store.DB) *Knowledge { return &Knowledge{db: db} }

// Learn записывает свидетельство. Возвращает false, если этот источник уже
// свидетельствовал об этом факте: ключ таблицы — пара (факт, источник).
func (k *Knowledge) Learn(f store.FactID, from store.EntityID) bool {
	for _, row := range k.db.Knowledge {
		if row.FactID == f && row.LearnedFrom == from {
			return false
		}
	}
	k.tick++
	k.db.Knowledge = append(k.db.Knowledge, store.Knowledge{
		FactID:      f,
		LearnedFrom: from,
		Confidence:  SourceConfidence,
		LearnedAt:   k.tick,
	})
	return true
}

func (k *Knowledge) Knows(f store.FactID) bool { return len(k.Sources(f)) > 0 }

// Sources возвращает источники в хронологическом порядке получения.
func (k *Knowledge) Sources(f store.FactID) []store.EntityID {
	rows := make([]store.Knowledge, 0, 4)
	for _, row := range k.db.Knowledge {
		if row.FactID == f {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].LearnedAt < rows[j].LearnedAt })
	out := make([]store.EntityID, len(rows))
	for i, r := range rows {
		out[i] = r.LearnedFrom
	}
	return out
}

// independent обходит источники в хронологическом порядке и оставляет те,
// у которых нет ребра ни с одним уже засчитанным. Обход детерминированный:
// перебора максимальных независимых множеств здесь нет и не нужно.
func (k *Knowledge) independent(f store.FactID) []store.EntityID {
	var kept []store.EntityID
	for _, cand := range k.Sources(f) {
		conflict := false
		for _, got := range kept {
			if k.db.Related(cand, got) {
				conflict = true
				break
			}
		}
		if !conflict {
			kept = append(kept, cand)
		}
	}
	return kept
}

// Confidence складывает независимые источники как 1 - П(1 - p).
func (k *Knowledge) Confidence(f store.FactID) float64 {
	product := 1.0
	for range k.independent(f) {
		product *= 1 - SourceConfidence
	}
	if product == 1.0 {
		return 0
	}
	return 1 - product
}

func (k *Knowledge) Corroborated(f store.FactID) bool {
	return k.Confidence(f) >= CorroborationThreshold
}

// TopicBank — единственный источник тем для вопросов. Спросить о факте,
// которого парти не знает, нельзя: это защита от угадывания.
func (k *Knowledge) TopicBank() []store.FactID {
	seen := map[store.FactID]bool{}
	var out []store.FactID
	for _, row := range k.db.Knowledge {
		if !seen[row.FactID] {
			seen[row.FactID] = true
			out = append(out, row.FactID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
```

- [ ] **Step 4: Прогнать тест, убедиться что проходит**

Run: `go test ./core/ -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add core/knowledge.go core/knowledge_test.go
git commit -m "feat(core): party_knowledge, банк тем и корроборация по независимым источникам"
```

---

### Task 8: Часы давления и их последствия

**Files:**
- Create: `core/clocks.go`
- Test: `core/clocks_test.go`

**Interfaces:**
- Consumes: `store.DB`, `store.Clock`, `store.Consequence` (Tasks 1–2)
- Produces: `core.Clocks` c `NewClocks(*store.DB) *Clocks`, `(*Clocks).Tick(store.ClockID, int) []store.Consequence`, `(*Clocks).TickAll(int) []store.Consequence`, `(*Clocks).Filled(store.ClockID) bool`, `(*Clocks).Snapshot() []store.Clock`

- [ ] **Step 1: Написать падающий тест**

`core/clocks_test.go`:

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func clocksDB() *store.DB {
	db := store.NewDB()
	db.Clocks["c_suspicion"] = &store.Clock{
		ID: "c_suspicion", Name: "Подозрение", Segments: 4, TickPolicy: "on_cost",
		OnFill: store.Consequence{FlavourKey: "clock.suspicion.filled", HostileTo: []store.EntityID{"e_toke"}},
	}
	return db
}

func TestTickFillsAndFiresOnce(t *testing.T) {
	c := NewClocks(clocksDB())
	if got := c.Tick("c_suspicion", 3); len(got) != 0 {
		t.Fatalf("часы сработали раньше заполнения: %v", got)
	}
	fired := c.Tick("c_suspicion", 1)
	if len(fired) != 1 || fired[0].FlavourKey != "clock.suspicion.filled" {
		t.Fatalf("последствие не сработало на заполнении: %v", fired)
	}
	// Повторные тики переполненных часов последствие не повторяют.
	if got := c.Tick("c_suspicion", 5); len(got) != 0 {
		t.Errorf("последствие сработало повторно: %v", got)
	}
	if !c.Filled("c_suspicion") {
		t.Error("часы не отмечены как заполненные")
	}
}

func TestTickNeverExceedsSegments(t *testing.T) {
	c := NewClocks(clocksDB())
	c.Tick("c_suspicion", 99)
	snap := c.Snapshot()
	if snap[0].Filled != snap[0].Segments {
		t.Errorf("filled=%d при segments=%d — счётчик убежал", snap[0].Filled, snap[0].Segments)
	}
}

func TestTickOnUnknownClockIsNoop(t *testing.T) {
	c := NewClocks(clocksDB())
	if got := c.Tick("c_nonexistent", 1); len(got) != 0 {
		t.Errorf("тик несуществующих часов дал последствие: %v", got)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run TestTick -v`
Expected: FAIL — `undefined: NewClocks`

- [ ] **Step 3: Написать `core/clocks.go`**

```go
package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

// Clocks — часы давления. Заполнение не проигрыш: оно меняет мир через
// Consequence, а mandatory-путь к каждому факту сохраняется по построению дела.
type Clocks struct {
	db    *store.DB
	fired map[store.ClockID]bool
}

func NewClocks(db *store.DB) *Clocks {
	return &Clocks{db: db, fired: map[store.ClockID]bool{}}
}

// Tick продвигает часы на n сегментов и возвращает последствия, сработавшие
// именно сейчас. Заполненные часы больше не срабатывают.
func (c *Clocks) Tick(id store.ClockID, n int) []store.Consequence {
	cl, ok := c.db.Clocks[id]
	if !ok {
		return nil
	}
	if cl.Filled >= cl.Segments {
		return nil
	}
	cl.Filled += n
	if cl.Filled > cl.Segments {
		cl.Filled = cl.Segments
	}
	if cl.Filled < cl.Segments || c.fired[id] {
		return nil
	}
	c.fired[id] = true
	return []store.Consequence{cl.OnFill}
}

// TickAll продвигает все часы с политикой on_cost — цена провала бьёт по
// каждому из них, если дело не указало иного.
func (c *Clocks) TickAll(n int) []store.Consequence {
	var out []store.Consequence
	for _, cl := range c.Snapshot() {
		if cl.TickPolicy == "on_cost" {
			out = append(out, c.Tick(cl.ID, n)...)
		}
	}
	return out
}

func (c *Clocks) Filled(id store.ClockID) bool {
	cl, ok := c.db.Clocks[id]
	return ok && cl.Filled >= cl.Segments
}

// Snapshot возвращает часы в стабильном порядке по ID — для вывода и для
// детерминированного обхода.
func (c *Clocks) Snapshot() []store.Clock {
	out := make([]store.Clock, 0, len(c.db.Clocks))
	for _, cl := range c.db.Clocks {
		out = append(out, *cl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
```

- [ ] **Step 4: Прогнать тест, убедиться что проходит**

Run: `go test ./core/ -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add core/clocks.go core/clocks_test.go
git commit -m "feat(core): часы давления с однократным срабатыванием последствия"
```

---

### Task 9: Лист персонажа, порог и ситуативные модификаторы

**Files:**
- Create: `rules/threshold/sheet.go`, `rules/threshold/mods.go`
- Test: `rules/threshold/mods_test.go`

**Interfaces:**
- Consumes: `core.Intent`, `core.SceneView`, `core.Verbs` (Tasks 4–5)
- Produces: `threshold.Sheet{Attrs map[string]int, Tags []Tag, Archetype string, Abilities []string}`, `threshold.Tag{Name, Verbs, NodeTags}`, `threshold.ParseSheet(json.RawMessage) (Sheet, error)`, `(Sheet).Attr(core.Verb) int`, `(Sheet).TagBonus(core.Verb, core.SceneView) int`, `threshold.ThresholdFor(string) int`, `threshold.Situational(core.Intent, core.SceneView) int`, константы `ThresholdEasy=10`, `ThresholdNormal=14`, `ThresholdHard=18`, `SituationalCap=4`

**Решение, зафиксированное здесь:** строка «Темнота, дождь, толпа | −2» даёт −2 **однократно**, если присутствует хотя бы один из этих тегов, а не −2 за каждый. Иначе одна дождливая ночная сцена съедает весь бюджет модификаторов и правила перестают быть настраиваемыми.

- [ ] **Step 1: Написать падающий тест**

`rules/threshold/mods_test.go`:

```go
package threshold

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

func TestThresholdIsClosedSet(t *testing.T) {
	cases := map[string]int{"easy": 10, "normal": 14, "hard": 18, "": 14, "нечто": 14}
	for in, want := range cases {
		if got := ThresholdFor(in); got != want {
			t.Errorf("ThresholdFor(%q) = %d, ожидалось %d", in, got, want)
		}
	}
}

func TestSituationalCountsFactorsAndClamps(t *testing.T) {
	cases := []struct {
		name string
		in   core.Intent
		view core.SceneView
		want int
	}{
		{"пусто", core.Intent{}, core.SceneView{}, 0},
		{"укрытие", core.Intent{}, core.SceneView{Cover: true}, 2},
		{"не обнаружен", core.Intent{}, core.SceneView{Undetected: true}, 2},
		{"превосходство", core.Intent{}, core.SceneView{Allies: 3, Foes: 1}, 2},
		{"меньшинство", core.Intent{}, core.SceneView{Allies: 1, Foes: 3}, -2},
		{"паритет", core.Intent{}, core.SceneView{Allies: 2, Foes: 2}, 0},
		{"темнота", core.Intent{}, core.SceneView{NodeTags: []string{"dark"}}, -2},
		{"темнота и дождь считаются один раз", core.Intent{},
			core.SceneView{NodeTags: []string{"dark", "rain"}}, -2},
		{"ранения", core.Intent{}, core.SceneView{Harm: 2}, -4},
		{"tier выше", core.Intent{}, core.SceneView{ActorTier: 2, TargetTier: 1}, 2},
		{"tier ниже", core.Intent{}, core.SceneView{ActorTier: 1, TargetTier: 2}, -2},
		{"инструмент", core.Intent{Args: core.Args{Item: "crowbar"}},
			core.SceneView{Tools: []string{"crowbar"}}, 2},
		{"инструмента нет в руках", core.Intent{Args: core.Args{Item: "crowbar"}},
			core.SceneView{}, 0},
		{"верхний кламп", core.Intent{},
			core.SceneView{Cover: true, Undetected: true, Allies: 3, Foes: 1, ActorTier: 2}, 4},
		{"нижний кламп", core.Intent{},
			core.SceneView{Harm: 3, NodeTags: []string{"dark"}, Allies: 1, Foes: 4}, -4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Situational(c.in, c.view); got != c.want {
				t.Errorf("Situational = %d, ожидалось %d", got, c.want)
			}
		})
	}
}

func TestTagAppliesByListNotBySense(t *testing.T) {
	s := Sheet{Tags: []Tag{{Name: "портовый", Verbs: []string{"question", "search"}}}}
	if got := s.TagBonus("question", core.SceneView{}); got != 2 {
		t.Errorf("тег не применился к глаголу из списка: %d", got)
	}
	if got := s.TagBonus("strike", core.SceneView{}); got != 0 {
		t.Errorf("тег применился к глаголу вне списка: %d", got)
	}
}

func TestTagAppliesByNodeTag(t *testing.T) {
	s := Sheet{Tags: []Tag{{Name: "ночной", NodeTags: []string{"dark"}}}}
	if got := s.TagBonus("strike", core.SceneView{NodeTags: []string{"dark"}}); got != 2 {
		t.Errorf("тег обстановки не применился: %d", got)
	}
	if got := s.TagBonus("strike", core.SceneView{NodeTags: []string{"crowd"}}); got != 0 {
		t.Errorf("тег обстановки применился не к той сцене: %d", got)
	}
}

func TestTagBonusNeverStacks(t *testing.T) {
	// Два подходящих тега дают +2, а не +4: бонус за теги ограничен.
	s := Sheet{Tags: []Tag{
		{Name: "портовый", Verbs: []string{"question"}},
		{Name: "дознаватель", Verbs: []string{"question"}},
	}}
	if got := s.TagBonus("question", core.SceneView{}); got != 2 {
		t.Errorf("TagBonus = %d, ожидалось 2", got)
	}
}

func TestAttrMapsVerbClassToAttribute(t *testing.T) {
	s := Sheet{Attrs: map[string]int{"body": 1, "edge": 0, "mind": 3, "will": 0}}
	if got := s.Attr("question"); got != 3 {
		t.Errorf("investigate должен читать mind: %d", got)
	}
	if got := s.Attr("strike"); got != 1 {
		t.Errorf("attack должен читать body: %d", got)
	}
	if got := s.Attr("persuade"); got != 0 {
		t.Errorf("social должен читать will: %d", got)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./rules/threshold/ -v`
Expected: FAIL — `undefined: ThresholdFor`

- [ ] **Step 3: Написать `rules/threshold/sheet.go`**

```go
// Package threshold — система правил «Порог». Заменяемая часть проекта:
// ядро видит её только через интерфейс core.RuleSystem.
package threshold

import (
	"encoding/json"

	"github.com/kliuchnikovv/dnd/core"
)

// Tag — тег персонажа. Применимость проверяется по спискам глаголов и тегов
// обстановки, а не по смыслу: семантическое суждение здесь недопустимо.
type Tag struct {
	Name     string   `json:"name"`
	Verbs    []string `json:"verbs"`
	NodeTags []string `json:"node_tags"`
}

type Sheet struct {
	Archetype string         `json:"archetype"`
	Attrs     map[string]int `json:"attrs"` // body, edge, mind, will
	Tags      []Tag          `json:"tags"`
	Abilities []string       `json:"abilities"`
}

func ParseSheet(raw json.RawMessage) (Sheet, error) {
	var s Sheet
	if len(raw) == 0 {
		return Sheet{Attrs: map[string]int{}}, nil
	}
	err := json.Unmarshal(raw, &s)
	if s.Attrs == nil {
		s.Attrs = map[string]int{}
	}
	return s, err
}

// classAttr связывает класс глагола с атрибутом. Таблица закрыта: новый класс
// без строки здесь читает нулевой атрибут, а не «что-нибудь похожее».
var classAttr = map[core.VerbClass]string{
	core.ClassInvestigate: "mind",
	core.ClassReason:      "mind",
	core.ClassResource:    "mind",
	core.ClassSocial:      "will",
	core.ClassSupport:     "will",
	core.ClassMove:        "edge",
	core.ClassSkill:       "edge",
	core.ClassAttack:      "body",
}

func (s Sheet) Attr(v core.Verb) int {
	def, ok := core.Verbs[v]
	if !ok {
		return 0
	}
	return s.Attrs[classAttr[def.Class]]
}

// TagBonus даёт +2, если хотя бы один тег подходит. Теги не складываются.
func (s Sheet) TagBonus(v core.Verb, view core.SceneView) int {
	for _, tag := range s.Tags {
		for _, tv := range tag.Verbs {
			if core.Verb(tv) == v {
				return TagValue
			}
		}
		for _, nt := range tag.NodeTags {
			if view.HasTag(nt) {
				return TagValue
			}
		}
	}
	return 0
}
```

- [ ] **Step 4: Написать `rules/threshold/mods.go`**

```go
package threshold

import "github.com/kliuchnikovv/dnd/core"

const (
	ThresholdEasy   = 10
	ThresholdNormal = 14
	ThresholdHard   = 18

	TagValue       = 2
	SituationalCap = 4
)

// ThresholdFor переводит сложность из данных дела в число. Множество закрыто:
// всё незнакомое — «Норма».
func ThresholdFor(gate string) int {
	switch gate {
	case "easy":
		return ThresholdEasy
	case "hard":
		return ThresholdHard
	default:
		return ThresholdNormal
	}
}

// adverseTags — обстановка, мешающая действию. Присутствие любого из них даёт
// -2 однократно, а не -2 за каждый.
var adverseTags = []string{"dark", "rain", "crowd"}

// Situational вычисляется, не оценивается: ни одного «на усмотрение мастера».
// Сумма клампится в ±SituationalCap.
func Situational(in core.Intent, view core.SceneView) int {
	sum := 0

	switch {
	case view.Allies > view.Foes:
		sum += 2
	case view.Allies < view.Foes:
		sum -= 2
	}
	if view.Cover {
		sum += 2
	}
	if view.Undetected {
		sum += 2
	}
	for _, tag := range adverseTags {
		if view.HasTag(tag) {
			sum -= 2
			break
		}
	}
	sum -= 2 * view.Harm
	switch {
	case view.ActorTier > view.TargetTier:
		sum += 2
	case view.ActorTier < view.TargetTier:
		sum -= 2
	}
	if in.Args.Item != "" && view.HasTool(in.Args.Item) {
		sum += 2
	}

	if sum > SituationalCap {
		return SituationalCap
	}
	if sum < -SituationalCap {
		return -SituationalCap
	}
	return sum
}
```

- [ ] **Step 5: Прогнать тест, убедиться что проходит**

Run: `go test ./rules/threshold/ -v`
Expected: PASS

- [ ] **Step 6: Коммит**

```bash
git add rules/threshold/sheet.go rules/threshold/mods.go rules/threshold/mods_test.go
git commit -m "feat(rules): лист персонажа, пороги и вычисляемые ситуативные модификаторы"
```

---

### Task 10: Resolve и точный тест распределения

**Files:**
- Create: `rules/threshold/resolve.go`
- Test: `rules/threshold/resolve_test.go`, `rules/threshold/distribution_test.go`

**Interfaces:**
- Consumes: Task 9, `core.Resolution`, `core.Dice`
- Produces: `threshold.System`, `threshold.New() *System`, `(*System).Resolve(core.Intent, core.SceneView, core.Dice) core.Resolution` — реализация `core.RuleSystem`

**Решение, зафиксированное здесь:** grit тратится автоматически на конвертацию выпавшего Провала в Частично, когда `view.Grit > 0`. Второй режим из спеки — «+2 до броска» — требует команды, которой нет в закрытом списке CLI §10 спеки, и переносится в M1b вместе с расширением CLI. Автоконвертация детерминирована, тестируема и не выдумывает игроку новых кнопок.

- [ ] **Step 1: Написать падающий тест классов исхода**

`rules/threshold/resolve_test.go`:

```go
package threshold

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

func sheetRaw(t *testing.T) core.SceneView {
	t.Helper()
	return core.SceneView{
		GateThreshold: "normal",
		Sheet:         []byte(`{"attrs":{"body":0,"edge":0,"mind":4,"will":0},"tags":[]}`),
	}
}

func TestMarginBoundsPickClass(t *testing.T) {
	// Порог 14, mind +4: маржа = d20 - 10.
	cases := []struct {
		die   int
		want  core.Outcome
		margin int
	}{
		{15, core.OutcomeCrit, 5},
		{14, core.OutcomeSuccess, 4},
		{10, core.OutcomeSuccess, 0},
		{9, core.OutcomePartial, -1},
		{6, core.OutcomePartial, -4},
		{5, core.OutcomeFail, -5},
		{2, core.OutcomeFail, -8},
	}
	for _, c := range cases {
		got := New().Resolve(core.Intent{Verb: "question"}, sheetRaw(t), dice.Fixed(c.die))
		if got.Class != c.want {
			t.Errorf("d20=%d: класс %v, ожидался %v", c.die, got.Class, c.want)
		}
		if got.Margin != c.margin {
			t.Errorf("d20=%d: маржа %d, ожидалась %d", c.die, got.Margin, c.margin)
		}
	}
}

func TestNat20StepsUpAndNat1StepsDown(t *testing.T) {
	view := sheetRaw(t)
	view.GateThreshold = "hard" // порог 18, маржа = d20 - 14

	// d20=20 даёт маржу +6 (крит) и остаётся критом: выше ступени нет.
	if got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(20)); got.Class != core.OutcomeCrit {
		t.Errorf("нат-20: класс %v, ожидался КРИТ", got.Class)
	}
	// d20=1 даёт маржу -13 (провал) и остаётся провалом: ниже ступени нет.
	if got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(1)); got.Class != core.OutcomeFail {
		t.Errorf("нат-1: класс %v, ожидался ПРОВАЛ", got.Class)
	}
}

func TestNat20LiftsSuccessToCrit(t *testing.T) {
	// Порог 18, mind +4: d20=20 даёт маржу +6, уже крит. Нужен случай, где
	// нат-20 действительно поднимает: mind 0, порог 18 -> маржа +2 = успех.
	view := core.SceneView{GateThreshold: "hard",
		Sheet: []byte(`{"attrs":{"mind":0},"tags":[]}`)}
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(20))
	if got.Class != core.OutcomeCrit {
		t.Errorf("нат-20 не поднял УСПЕХ до КРИТА: %v (маржа %d)", got.Class, got.Margin)
	}
}

func TestGritConvertsFailToPartial(t *testing.T) {
	view := sheetRaw(t)
	view.Grit = 3
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(2))
	if got.Class != core.OutcomePartial {
		t.Fatalf("grit не сконвертировал провал: %v", got.Class)
	}
	var spent bool
	for _, m := range got.Mutations {
		if m.Kind == core.MutResource && m.Target == "grit" && m.Delta == -1 {
			spent = true
		}
	}
	if !spent {
		t.Error("grit сконвертировал провал, но не списался")
	}
}

func TestNoGritNoConversion(t *testing.T) {
	view := sheetRaw(t)
	view.Grit = 0
	got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(2))
	if got.Class != core.OutcomeFail {
		t.Errorf("провал сконвертирован без grit: %v", got.Class)
	}
}

func TestNonRollingVerbSucceedsWithoutDice(t *testing.T) {
	got := New().Resolve(core.Intent{Verb: "compare"}, sheetRaw(t), dice.Fixed(1))
	if got.Class != core.OutcomeSuccess {
		t.Errorf("compare дал %v — глагол без броска обязан просто удаваться", got.Class)
	}
	if got.Log.Die != 0 {
		t.Errorf("compare бросил кость: %d", got.Log.Die)
	}
}

func TestLogCarriesNamedTerms(t *testing.T) {
	got := New().Resolve(core.Intent{Verb: "question"}, sheetRaw(t), dice.Fixed(11))
	if got.Log.Die != 11 || got.Log.Threshold != 14 {
		t.Errorf("лог броска неполон: %+v", got.Log)
	}
	names := map[string]int{}
	for _, term := range got.Log.Terms {
		names[term.Name] = term.Value
	}
	if names["атрибут"] != 4 {
		t.Errorf("в логе нет слагаемого «атрибут»: %+v", got.Log.Terms)
	}
}

func TestSystemSatisfiesRuleSystem(t *testing.T) {
	var _ core.RuleSystem = New()
}
```

- [ ] **Step 2: Написать точный тест распределения**

`rules/threshold/distribution_test.go`. Двадцать граней перечислимы, поэтому доля считается точно — тест никогда не флакает.

```go
package threshold

import (
	"fmt"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

func TestOutcomeDistributionMatchesSpec(t *testing.T) {
	cases := []struct {
		mod                              int
		gate                             string
		successPlus, partial, cleanFail  int // из 20 граней
	}{
		{4, "normal", 11, 4, 5},  // 55% / 20% / 25%
		{6, "normal", 13, 4, 3},  // 65% / 20% / 15%
		{4, "hard", 7, 4, 9},     // 35% / 20% / 45%
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("mod%+d_%s", c.mod, c.gate), func(t *testing.T) {
			view := core.SceneView{
				GateThreshold: c.gate,
				Grit:          0, // конвертация grit исказила бы распределение
				Sheet:         []byte(fmt.Sprintf(`{"attrs":{"mind":%d},"tags":[]}`, c.mod)),
			}
			var successPlus, partial, fail int
			for die := 1; die <= 20; die++ {
				got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(die))
				switch got.Class {
				case core.OutcomeCrit, core.OutcomeSuccess:
					successPlus++
				case core.OutcomePartial:
					partial++
				default:
					fail++
				}
			}
			if successPlus != c.successPlus || partial != c.partial || fail != c.cleanFail {
				t.Errorf("распределение %d/%d/%d из 20, ожидалось %d/%d/%d",
					successPlus, partial, fail, c.successPlus, c.partial, c.cleanFail)
			}
		})
	}
}

func TestPartialStaysNearTwentyPercentAcrossModifiers(t *testing.T) {
	// Окно «частично» шириной 4 пункта не зависит от прокачки — это фича:
	// доля fail-forward постоянна.
	for mod := 0; mod <= 8; mod++ {
		view := core.SceneView{
			GateThreshold: "normal",
			Sheet:         []byte(fmt.Sprintf(`{"attrs":{"mind":%d},"tags":[]}`, mod)),
		}
		partial := 0
		for die := 1; die <= 20; die++ {
			if New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(die)).Class == core.OutcomePartial {
				partial++
			}
		}
		if partial != 4 {
			t.Errorf("модификатор +%d: частично %d/20, ожидалось 4/20", mod, partial)
		}
	}
}
```

- [ ] **Step 3: Прогнать тесты, убедиться что падают**

Run: `go test ./rules/threshold/ -run 'TestMargin|TestNat|TestGrit|TestOutcomeDistribution' -v`
Expected: FAIL — `undefined: New`

- [ ] **Step 4: Написать `rules/threshold/resolve.go`**

```go
package threshold

import "github.com/kliuchnikovv/dnd/core"

// System реализует core.RuleSystem. Resolve — чистая функция от
// (Intent, SceneView, Dice): никакого состояния между вызовами.
type System struct{}

func New() *System { return &System{} }

func (s *System) Resolve(in core.Intent, view core.SceneView, d core.Dice) core.Resolution {
	def := core.Verbs[in.Verb]

	// Глагол без броска просто удаётся. Кость не трогаем: сдвиг стрима здесь
	// сломал бы воспроизводимость прогона по seed.
	if !def.Rolls {
		return core.Resolution{Class: core.OutcomeSuccess, Margin: 0}
	}

	sheet, _ := ParseSheet(view.Sheet)
	attr := sheet.Attr(in.Verb)
	tag := sheet.TagBonus(in.Verb, view)
	sit := Situational(in, view)
	th := ThresholdFor(view.GateThreshold)

	die := d.D20()
	total := die + attr + tag + sit
	margin := total - th
	class := classify(margin)

	switch die {
	case 20:
		class = class.Up()
	case 1:
		class = class.Down()
	}

	res := core.Resolution{
		Class:  class,
		Margin: margin,
		Log: core.RollLog{
			Die: die, Total: total, Threshold: th,
			Terms: []core.RollTerm{
				{Name: "атрибут", Value: attr},
				{Name: "тег", Value: tag},
				{Name: "ситуация", Value: sit},
			},
		},
	}

	// grit конвертирует выпавший Провал в Частично, тратя одно очко.
	if res.Class == core.OutcomeFail && view.Grit > 0 {
		res.Class = core.OutcomePartial
		res.Mutations = append(res.Mutations,
			core.Mutation{Kind: core.MutResource, Target: "grit", Delta: -1})
	}

	res.Costs = costFor(def.Class, res.Class, res.Margin)
	return res
}

func classify(margin int) core.Outcome {
	switch {
	case margin >= 5:
		return core.OutcomeCrit
	case margin >= 0:
		return core.OutcomeSuccess
	case margin >= -4:
		return core.OutcomePartial
	default:
		return core.OutcomeFail
	}
}
```

- [ ] **Step 5: Написать заглушку `costFor`, чтобы пакет собрался**

Полная реализация — Task 11. Здесь минимум, ровно чтобы компилировалось и тесты этой задачи прошли. Добавить в конец `rules/threshold/resolve.go`:

```go
// costFor заменяется полной таблицей в rules/threshold/cost.go (Task 11).
func costFor(core.VerbClass, core.Outcome, int) []core.CostKind { return nil }
```

- [ ] **Step 6: Прогнать тесты, убедиться что проходят**

Run: `go test ./rules/threshold/ -v`
Expected: PASS, включая `TestOutcomeDistributionMatchesSpec` со всеми тремя строками таблицы

- [ ] **Step 7: Коммит**

```bash
git add rules/threshold/resolve.go rules/threshold/resolve_test.go rules/threshold/distribution_test.go
git commit -m "feat(rules): Resolve с классами исхода, нат-20/нат-1 и точный тест распределения"
```

---

### Task 11: Выбор цены провала по классу глагола

**Files:**
- Create: `rules/threshold/cost.go`
- Modify: `rules/threshold/resolve.go` — удалить заглушку `costFor` из Task 10
- Test: `rules/threshold/cost_test.go`

**Interfaces:**
- Consumes: `core.CostKind`, `core.VerbClass`, `core.Outcome`
- Produces: `costFor(core.VerbClass, core.Outcome, int) []core.CostKind` (не экспортируется — вызывается только из `Resolve`)

- [ ] **Step 1: Написать падающий тест**

`rules/threshold/cost_test.go`:

```go
package threshold

import (
	"reflect"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

func TestCostTableCoversEveryClass(t *testing.T) {
	classes := []core.VerbClass{
		core.ClassInvestigate, core.ClassReason, core.ClassSocial, core.ClassMove,
		core.ClassAttack, core.ClassSupport, core.ClassResource, core.ClassSkill,
	}
	for _, cl := range classes {
		for _, out := range []core.Outcome{core.OutcomePartial, core.OutcomeFail} {
			got := costFor(cl, out, -3)
			if cl == core.ClassReason {
				if len(got) != 0 {
					t.Errorf("%s/%v: рассуждение не имеет цены, получено %v", cl, out, got)
				}
				continue
			}
			if len(got) == 0 {
				t.Errorf("%s/%v: цена не определена — дыра в таксономии", cl, out)
			}
			for _, c := range got {
				if !validCost(c) {
					t.Errorf("%s/%v: цена %q вне таксономии ядра", cl, out, c)
				}
			}
		}
	}
}

func TestSuccessAndCritCostNothing(t *testing.T) {
	for _, out := range []core.Outcome{core.OutcomeSuccess, core.OutcomeCrit} {
		if got := costFor(core.ClassInvestigate, out, 3); len(got) != 0 {
			t.Errorf("успех стоил %v", got)
		}
	}
}

func TestSpecificCostsMatchTable(t *testing.T) {
	cases := []struct {
		class core.VerbClass
		out   core.Outcome
		want  []core.CostKind
	}{
		{core.ClassInvestigate, core.OutcomePartial, []core.CostKind{core.CostTickClock}},
		{core.ClassInvestigate, core.OutcomeFail, []core.CostKind{core.CostFalseLead}},
		{core.ClassSocial, core.OutcomePartial, []core.CostKind{core.CostDebt}},
		{core.ClassSocial, core.OutcomeFail, []core.CostKind{core.CostTickClock, core.CostDispositionDown}},
		{core.ClassMove, core.OutcomePartial, []core.CostKind{core.CostPositionWorse}},
		{core.ClassMove, core.OutcomeFail, []core.CostKind{core.CostTickClock}},
		{core.ClassAttack, core.OutcomePartial, []core.CostKind{core.CostTickClock}},
		{core.ClassAttack, core.OutcomeFail, []core.CostKind{core.CostHarmSelf}},
		{core.ClassSupport, core.OutcomePartial, []core.CostKind{core.CostHalfEffect}},
		{core.ClassSupport, core.OutcomeFail, []core.CostKind{core.CostHarmSelf}},
		{core.ClassResource, core.OutcomePartial, []core.CostKind{core.CostResourceSpent}},
		{core.ClassResource, core.OutcomeFail, []core.CostKind{core.CostResourceSpent}},
		{core.ClassSkill, core.OutcomePartial, []core.CostKind{core.CostPositionWorse}},
		{core.ClassSkill, core.OutcomeFail, []core.CostKind{core.CostTickClock, core.CostPositionWorse}},
	}
	for _, c := range cases {
		got := costFor(c.class, c.out, -3)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s/%v: %v, ожидалось %v", c.class, c.out, got, c.want)
		}
	}
}

func TestCatastrophicMarginDoublesCost(t *testing.T) {
	normal := costFor(core.ClassSocial, core.OutcomeFail, -9)
	doubled := costFor(core.ClassSocial, core.OutcomeFail, -10)
	if len(doubled) != 2*len(normal) {
		t.Fatalf("маржа -10 дала %d элементов, ожидалось %d", len(doubled), 2*len(normal))
	}
	if !reflect.DeepEqual(doubled[:len(normal)], normal) {
		t.Error("удвоение исказило состав цены")
	}
	if !reflect.DeepEqual(doubled[len(normal):], normal) {
		t.Error("вторая половина удвоенной цены не совпадает с первой")
	}
}

func TestReasonNeverCostsEvenOnCatastrophe(t *testing.T) {
	if got := costFor(core.ClassReason, core.OutcomeFail, -20); len(got) != 0 {
		t.Errorf("рассуждение стоило %v", got)
	}
}

func validCost(c core.CostKind) bool {
	for _, k := range core.AllCostKinds() {
		if k == c {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./rules/threshold/ -run TestCost -v`
Expected: FAIL — заглушка возвращает `nil`, тест сообщает «дыра в таксономии»

- [ ] **Step 3: Удалить заглушку из `resolve.go`**

Удалить из `rules/threshold/resolve.go` строки:

```go
// costFor заменяется полной таблицей в rules/threshold/cost.go (Task 11).
func costFor(core.VerbClass, core.Outcome, int) []core.CostKind { return nil }
```

- [ ] **Step 4: Написать `rules/threshold/cost.go`**

```go
package threshold

import "github.com/kliuchnikovv/dnd/core"

// CatastrophicMargin — маржа, начиная с которой цена провала удваивается.
const CatastrophicMargin = -10

// costTable — выбор цены из таксономии ядра по классу глагола. Система правил
// только выбирает; исполняет ядро. Строка reason пуста намеренно: рассуждение
// не имеет цены.
var costTable = map[core.VerbClass]map[core.Outcome][]core.CostKind{
	core.ClassInvestigate: {
		core.OutcomePartial: {core.CostTickClock},
		core.OutcomeFail:    {core.CostFalseLead},
	},
	core.ClassReason: {},
	core.ClassSocial: {
		core.OutcomePartial: {core.CostDebt},
		core.OutcomeFail:    {core.CostTickClock, core.CostDispositionDown},
	},
	core.ClassMove: {
		core.OutcomePartial: {core.CostPositionWorse},
		core.OutcomeFail:    {core.CostTickClock},
	},
	core.ClassAttack: {
		core.OutcomePartial: {core.CostTickClock},
		core.OutcomeFail:    {core.CostHarmSelf},
	},
	core.ClassSupport: {
		core.OutcomePartial: {core.CostHalfEffect},
		core.OutcomeFail:    {core.CostHarmSelf},
	},
	core.ClassResource: {
		core.OutcomePartial: {core.CostResourceSpent},
		core.OutcomeFail:    {core.CostResourceSpent},
	},
	core.ClassSkill: {
		core.OutcomePartial: {core.CostPositionWorse},
		core.OutcomeFail:    {core.CostTickClock, core.CostPositionWorse},
	},
}

func costFor(class core.VerbClass, out core.Outcome, margin int) []core.CostKind {
	if out != core.OutcomePartial && out != core.OutcomeFail {
		return nil
	}
	base := costTable[class][out]
	if len(base) == 0 {
		return nil
	}
	out2 := make([]core.CostKind, 0, 2*len(base))
	out2 = append(out2, base...)
	if margin <= CatastrophicMargin {
		out2 = append(out2, base...)
	}
	return out2
}
```

- [ ] **Step 5: Прогнать тесты, убедиться что проходят**

Run: `go test ./rules/threshold/ -v`
Expected: PASS — включая распределение из Task 10, которое цена провала не меняет

- [ ] **Step 6: Коммит**

```bash
git add rules/threshold/cost.go rules/threshold/cost_test.go rules/threshold/resolve.go
git commit -m "feat(rules): полная таблица выбора цены провала с удвоением при марже <= -10"
```

---

### Task 12: Игровое состояние и исполнение мутаций и цены

**Files:**
- Create: `core/game.go`, `core/mutate.go`
- Test: `core/mutate_test.go`

**Interfaces:**
- Consumes: Tasks 2, 5, 6, 7, 8
- Produces: `core.Config{DB, Rules, Dice, Truth, Flavour, Start, Actor, Tokens}`, `core.Game`, `core.NewGame(Config) *Game`, поля `(*Game).Node`, `(*Game).Attempts`, `(*Game).Detected`, `(*Game).Disposition map[store.EntityID]int`, `(*Game).Debts map[store.EntityID]int`, `(*Game).K *Knowledge`, `(*Game).C *Clocks`; методы `(*Game).applyMutations([]Mutation)`, `(*Game).executeCosts([]CostKind, Intent) []store.Consequence`, `(*Game).applyConsequences([]store.Consequence)`, `(*Game).Flavour(string) string`

- [ ] **Step 1: Написать падающий тест**

`core/mutate_test.go`:

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

type nilRules struct{}

func (nilRules) Resolve(Intent, SceneView, Dice) Resolution { return Resolution{} }

type nilDice struct{}

func (nilDice) D20() int          { return 10 }
func (nilDice) Roll(int, int) int { return 3 }

func testGame() *Game {
	db := store.NewDB()
	db.Entities["e_toke"] = store.Entity{ID: "e_toke", Kind: store.EntityNPC, Node: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3, Harm: 0}
	db.Clocks["c_suspicion"] = &store.Clock{
		ID: "c_suspicion", Segments: 2, TickPolicy: "on_cost",
		OnFill: store.Consequence{FlavourKey: "clock.filled", HostileTo: []store.EntityID{"e_toke"}},
	}
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	return NewGame(Config{
		DB:      db,
		Rules:   nilRules{},
		Dice:    nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{"clock.filled": "Прилив забрал следы."},
		Start:   "n_quay",
		Actor:   "pc",
	})
}

func TestMutationsAreAppliedByName(t *testing.T) {
	g := testGame()
	g.applyMutations([]Mutation{
		{Kind: MutResource, Target: "grit", Delta: -1},
		{Kind: MutHarm, Target: "pc", Delta: 1},
		{Kind: MutDisposition, Target: "e_toke", Delta: -1},
	})
	if got := g.DB.Characters["pc"].Grit; got != 2 {
		t.Errorf("grit = %d, ожидалось 2", got)
	}
	if got := g.DB.Characters["pc"].Harm; got != 1 {
		t.Errorf("harm = %d, ожидалось 1", got)
	}
	if got := g.Disposition["e_toke"]; got != -1 {
		t.Errorf("disposition = %d, ожидалось -1", got)
	}
}

func TestUnknownResourceIsIgnoredNotPanicked(t *testing.T) {
	// Имена ресурсов даёт система правил. Ядро их не интерпретирует и не
	// обязано знать: незнакомое имя не должно ронять прогон.
	g := testGame()
	g.applyMutations([]Mutation{{Kind: MutResource, Target: "slot_3", Delta: -1}})
	if g.DB.Characters["pc"].Grit != 3 {
		t.Error("незнакомый ресурс задел grit")
	}
}

func TestEveryCostKindExecutes(t *testing.T) {
	in := Intent{Verb: "question", Args: Args{Target: "e_toke"}}
	for _, kind := range AllCostKinds() {
		g := testGame()
		before := snapshot(g)
		g.executeCosts([]CostKind{kind}, in)
		if kind == CostHalfEffect || kind == CostResourceSpent || kind == CostFalseLead {
			// Эти три меняют не состояние мира, а исход хода: их эффект
			// проверяется в TurnResult (Task 13).
			continue
		}
		if snapshot(g) == before {
			t.Errorf("цена %q не изменила состояние — не исполнена", kind)
		}
	}
}

func TestTickClockCostFiresConsequence(t *testing.T) {
	g := testGame()
	in := Intent{Verb: "question", Args: Args{Target: "e_toke"}}
	if got := g.executeCosts([]CostKind{CostTickClock}, in); len(got) != 0 {
		t.Fatalf("часы сработали на первом тике из двух: %v", got)
	}
	got := g.executeCosts([]CostKind{CostTickClock}, in)
	if len(got) != 1 || got[0].FlavourKey != "clock.filled" {
		t.Fatalf("последствие не сработало: %v", got)
	}
	g.applyConsequences(got)
	if g.Disposition["e_toke"] >= 0 {
		t.Error("HostileTo не понизил расположение")
	}
}

func TestFlavourFallsBackToKey(t *testing.T) {
	g := testGame()
	if got := g.Flavour("clock.filled"); got != "Прилив забрал следы." {
		t.Errorf("текст не найден: %q", got)
	}
	// Отсутствующий ключ виден сразу, а не молча превращается в пустую строку.
	if got := g.Flavour("нет.такого.ключа"); got != "[нет.такого.ключа]" {
		t.Errorf("отсутствующий ключ = %q, ожидалось [нет.такого.ключа]", got)
	}
}

func snapshot(g *Game) [4]int {
	c := g.DB.Characters["pc"]
	return [4]int{c.Grit, c.Harm, g.Disposition["e_toke"] + g.Debts["e_toke"],
		g.DB.Clocks["c_suspicion"].Filled + boolInt(g.Detected)}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run 'TestMutations|TestUnknownResource|TestEveryCostKind|TestTickClockCost|TestFlavour' -v`
Expected: FAIL — `undefined: NewGame`

- [ ] **Step 3: Написать `core/game.go`**

```go
package core

import (
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// TokenGrant связывает токен слота обвинения с фактом, который его открывает.
type TokenGrant struct {
	Slot  string      // "who" | "how" | "when" | "why"
	Token store.Token
	Fact  store.FactID
}

type Config struct {
	DB      *store.DB
	Rules   RuleSystem
	Dice    Dice
	Truth   accusation.Truth
	Flavour map[string]string
	Tokens  []TokenGrant
	Start   store.NodeID
	Actor   store.CharacterID
}

// Game — всё изменяемое состояние прогона. Ядро; системы правил здесь нет
// нигде, кроме поля Rules за интерфейсом.
type Game struct {
	DB    *store.DB
	Rules RuleSystem
	Dice  Dice
	K     *Knowledge
	C     *Clocks

	Node     store.NodeID
	Actor    store.CharacterID
	Attempts int
	Detected bool

	Disposition map[store.EntityID]int
	Debts       map[store.EntityID]int

	truth   accusation.Truth
	tokens  []TokenGrant
	flavour map[string]string
}

func NewGame(cfg Config) *Game {
	return &Game{
		DB: cfg.DB, Rules: cfg.Rules, Dice: cfg.Dice,
		K: NewKnowledge(cfg.DB), C: NewClocks(cfg.DB),
		Node: cfg.Start, Actor: cfg.Actor,
		Disposition: map[store.EntityID]int{},
		Debts:       map[store.EntityID]int{},
		truth:       cfg.Truth, tokens: cfg.Tokens, flavour: cfg.Flavour,
	}
}

// Flavour отдаёт текст по ключу. Отсутствующий ключ виден в выводе как
// [ключ]: пустая строка на его месте прячет дыру в деле до самого демо.
func (g *Game) Flavour(key string) string {
	if s, ok := g.flavour[key]; ok {
		return s
	}
	return "[" + key + "]"
}
```

- [ ] **Step 4: Написать `core/mutate.go`**

```go
package core

import "github.com/kliuchnikovv/dnd/store"

// applyMutations применяет обобщённые мутации. Ядро знает имена только двух
// ресурсов — grit и harm, потому что на них ссылается таксономия цены провала.
// Всё прочее приходит от системы правил и молча игнорируется, если ядру
// незнакомо: это не ошибка, а граница ответственности.
func (g *Game) applyMutations(ms []Mutation) {
	ch := g.DB.Characters[g.Actor]
	for _, m := range ms {
		switch m.Kind {
		case MutResource:
			if m.Target == "grit" && ch != nil {
				ch.Grit += m.Delta
				if ch.Grit < 0 {
					ch.Grit = 0
				}
			}
		case MutHarm:
			if c, ok := g.DB.Characters[store.CharacterID(m.Target)]; ok {
				c.Harm += m.Delta
			}
		case MutClock:
			g.C.Tick(store.ClockID(m.Target), m.Delta)
		case MutDisposition:
			g.Disposition[store.EntityID(m.Target)] += m.Delta
		case MutPosition:
			g.Detected = m.Delta < 0
		}
	}
}

// executeCosts исполняет выбранную правилами цену. Возвращает последствия
// заполнившихся часов.
func (g *Game) executeCosts(cs []CostKind, in Intent) []store.Consequence {
	var fired []store.Consequence
	ch := g.DB.Characters[g.Actor]
	for _, c := range cs {
		switch c {
		case CostTickClock:
			fired = append(fired, g.C.TickAll(1)...)
		case CostDebt:
			g.Debts[in.Args.Target]++
		case CostDispositionDown:
			g.Disposition[in.Args.Target]--
		case CostPositionWorse:
			g.Detected = true
		case CostHarmSelf:
			if ch != nil {
				ch.Harm++
			}
		case CostFalseLead, CostHalfEffect, CostResourceSpent:
			// Меняют не мир, а исход хода: отражаются в TurnResult.
		}
	}
	return fired
}

// applyConsequences исполняет срабатывание часов. Держатели могут исчезнуть,
// сущности — озлобиться, но mandatory-путь к каждому факту дело обязано
// сохранить: это проверяет валидатор загрузки.
func (g *Game) applyConsequences(cs []store.Consequence) {
	for _, c := range cs {
		for _, f := range c.RemoveHolders {
			delete(g.DB.Holders, f)
		}
		for _, e := range c.HostileTo {
			g.Disposition[e] -= 2
		}
	}
}
```

- [ ] **Step 5: Прогнать тест, убедиться что проходит**

Run: `go test ./core/ -v`
Expected: PASS

- [ ] **Step 6: Коммит**

```bash
git add core/game.go core/mutate.go core/mutate_test.go
git commit -m "feat(core): игровое состояние, применение мутаций и исполнение цены провала"
```

---

### Task 13: `Apply` — пятишаговый ход

**Files:**
- Create: `core/turn.go`
- Test: `core/turn_test.go`

**Interfaces:**
- Consumes: Tasks 4–8, 12
- Produces: `core.TurnResult{Refused bool, Refusal string, FlavourKey string, Learned []Learned, Res *Resolution, Costs []CostKind, Fired []store.Consequence, FalseLead bool, HalfEffect bool}`, `core.Learned{Fact store.FactID, From store.EntityID}`, `(*Game).Apply(Intent) TurnResult`, `(*Game).SceneView(Intent) SceneView`, `(*Game).Available(store.FactID) bool`

- [ ] **Step 1: Написать падающий тест**

`core/turn_test.go`:

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

type fixedRules struct{ out Outcome }

func (r fixedRules) Resolve(in Intent, _ SceneView, _ Dice) Resolution {
	res := Resolution{Class: r.out, Margin: 0}
	if r.out == OutcomeFail {
		res.Margin = -6
		res.Costs = []CostKind{CostFalseLead}
	}
	return res
}

func turnGame(out Outcome) *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Adjacent: []store.NodeID{"n_forge"}}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Adjacent: []store.NodeID{"n_quay"}}
	db.Entities["e_toke"] = store.Entity{ID: "e_toke", Kind: store.EntityNPC, Node: "n_quay"}
	db.Entities["e_ivar"] = store.Entity{ID: "e_ivar", Kind: store.EntityNPC, Node: "n_forge"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_open"] = store.Fact{ID: "f_open", Key: "open"}
	db.Facts["f_gated"] = store.Fact{ID: "f_gated", Key: "gated"}
	db.Holders["f_open"] = []store.FactHolder{{
		FactID: "f_open", HolderID: "e_toke", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}
	db.Holders["f_gated"] = []store.FactHolder{{
		FactID: "f_gated", HolderID: "e_toke", Mandatory: false,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal",
			Requires: []store.FactID{"f_open"}},
	}}
	db.Clocks["c_suspicion"] = &store.Clock{ID: "c_suspicion", Segments: 6, TickPolicy: "on_cost"}
	return NewGame(Config{
		DB: db, Rules: fixedRules{out}, Dice: nilDice{},
		Truth: accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func TestUnknownTopicIsRefusedNotFailed(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	// f_gated парти не знает — темы для вопроса нет.
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_gated"}})
	if !got.Refused {
		t.Fatal("вопрос о неизвестном факте прошёл — защита от угадывания дырявая")
	}
	if got.Res != nil {
		t.Error("отказ дошёл до броска")
	}
	if g.DB.Clocks["c_suspicion"].Filled != 0 {
		t.Error("отказ тикнул часы — ход был потрачен")
	}
}

func TestAbsentTargetIsRefused(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_ivar", Topic: "f_open"}})
	if !got.Refused {
		t.Error("допрошен персонаж из другой локации")
	}
}

func TestWrongVerbForGateIsRefused(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	got := g.Apply(Intent{Verb: "search", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if !got.Refused {
		t.Error("факт выдан глаголом не из gate.verbs")
	}
}

func TestMandatoryFactBypassesTheRoll(t *testing.T) {
	// Кубик всегда проваливает — mandatory-факт обязан выдаться всё равно.
	g := turnGame(OutcomeFail)
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if got.Refused {
		t.Fatalf("mandatory-факт отвергнут: %s", got.Refusal)
	}
	if got.Res != nil {
		t.Error("mandatory-факт прошёл через бросок")
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_open" {
		t.Fatalf("факт не выдан: %v", got.Learned)
	}
	if !g.K.Knows("f_open") {
		t.Error("факт не записан в party_knowledge")
	}
}

func TestGatedFactNeedsItsRequirement(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	g.K.Learn("f_gated", "e_bern") // тема известна, но требование не выполнено
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_gated"}})
	if !got.Refused {
		t.Error("факт выдан без выполненного requires")
	}
}

func TestFailedRollLearnsNothingAndCosts(t *testing.T) {
	g := turnGame(OutcomeFail)
	g.K.Learn("f_gated", "e_bern")
	g.K.Learn("f_open", "e_bern") // выполняем requires
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_gated"}})
	if got.Refused {
		t.Fatalf("действие отвергнуто вместо провала: %s", got.Refusal)
	}
	for _, l := range got.Learned {
		if l.From == "e_toke" && l.Fact == "f_gated" {
			t.Error("провал выдал факт")
		}
	}
	if !got.FalseLead {
		t.Error("провал investigate не дал ложного следа")
	}
}

func TestSceneViewHidesFactsAndTruth(t *testing.T) {
	// Структурная гарантия: в SceneView нет полей под граф фактов и truth.
	g := turnGame(OutcomeSuccess)
	view := g.SceneView(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_open"}})
	if view.Node != "n_quay" {
		t.Errorf("узел в SceneView = %q", view.Node)
	}
	if view.GateThreshold != "normal" {
		t.Errorf("сложность gate не доехала до правил: %q", view.GateThreshold)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run 'TestUnknownTopic|TestAbsentTarget|TestWrongVerb|TestMandatory|TestGatedFact|TestFailedRoll|TestSceneViewHides' -v`
Expected: FAIL — `undefined: TurnResult`

- [ ] **Step 3: Написать `core/turn.go`**

```go
package core

import "github.com/kliuchnikovv/dnd/store"

type Learned struct {
	Fact store.FactID
	From store.EntityID
}

// TurnResult — исход хода. Отказ и провал различены намеренно: отказ не
// тратит ход и не тикает часы, и игрок обязан видеть разницу мгновенно.
type TurnResult struct {
	Refused    bool
	Refusal    string
	FlavourKey string
	Learned    []Learned
	Res        *Resolution
	Costs      []CostKind
	Fired      []store.Consequence
	FalseLead  bool
	HalfEffect bool
}

// Apply — пятишаговый ход: валидация, ветка без броска, сборка SceneView,
// Resolve, применение. Ядро никогда не видит кости: они уходят внутрь Resolve.
func (g *Game) Apply(in Intent) TurnResult {
	def, ok := Verbs[in.Verb]
	if !ok {
		return refuse("неизвестное действие")
	}

	// Шаг 1: валидация.
	if r, bad := g.validate(in, def); bad {
		return r
	}

	holder, found := g.holderFor(in)

	// Шаг 2: ветка без броска.
	if !def.Rolls || (found && holder.Mandatory) {
		res := TurnResult{FlavourKey: g.flavourKey(in)}
		if found {
			if g.K.Learn(holder.FactID, holder.HolderID) {
				res.Learned = append(res.Learned, Learned{holder.FactID, holder.HolderID})
			}
		}
		return res
	}

	// Шаг 3: сборка SceneView.
	view := g.SceneView(in)

	// Шаг 4: бросок в системе правил.
	resolution := g.Rules.Resolve(in, view, g.Dice)

	// Шаг 5: применение.
	out := TurnResult{Res: &resolution, Costs: resolution.Costs, FlavourKey: g.flavourKey(in)}
	g.applyMutations(resolution.Mutations)
	out.Fired = g.executeCosts(resolution.Costs, in)
	g.applyConsequences(out.Fired)

	for _, c := range resolution.Costs {
		switch c {
		case CostFalseLead:
			out.FalseLead = true
		case CostHalfEffect:
			out.HalfEffect = true
		}
	}

	if found && resolution.Class >= OutcomeSuccess {
		if g.K.Learn(holder.FactID, holder.HolderID) {
			out.Learned = append(out.Learned, Learned{holder.FactID, holder.HolderID})
		}
	}
	return out
}

func (g *Game) validate(in Intent, def VerbDef) (TurnResult, bool) {
	if in.Args.Target != "" {
		e, ok := g.DB.Entities[in.Args.Target]
		if !ok {
			return refuse("такой сущности в деле нет"), true
		}
		if e.Node != g.Node {
			return refuse("этого нет в текущей локации"), true
		}
	}
	if in.Args.Node != "" && !g.DB.Adjacent(g.Node, in.Args.Node) {
		return refuse("туда отсюда не пройти"), true
	}
	if in.Args.Topic != "" {
		// Банк тем строится из party_knowledge: спросить о неизвестном нельзя.
		if !g.K.Knows(in.Args.Topic) {
			return refuse("парти об этом ничего не знает — спрашивать не о чем"), true
		}
	}
	for _, f := range in.Args.Facts {
		if !g.K.Knows(f) {
			return refuse("этот факт парти неизвестен"), true
		}
	}
	if in.Args.Topic != "" {
		if _, ok := g.holderFor(in); !ok {
			return refuse("здесь об этом не расскажут"), true
		}
	}
	return TurnResult{}, false
}

// holderFor находит держателя, который может выдать запрошенный факт именно
// этим глаголом при выполненных требованиях.
func (g *Game) holderFor(in Intent) (store.FactHolder, bool) {
	if in.Args.Topic == "" {
		return store.FactHolder{}, false
	}
	for _, h := range g.DB.HoldersOf(in.Args.Topic) {
		if h.HolderID != in.Args.Target {
			continue
		}
		if !gateAllows(h.Gate, in.Verb) {
			continue
		}
		if !g.requirementsMet(h.Gate) {
			continue
		}
		return h, true
	}
	return store.FactHolder{}, false
}

func gateAllows(gate store.Gate, v Verb) bool {
	for _, gv := range gate.Verbs {
		if Verb(gv) == v {
			return true
		}
	}
	return false
}

func (g *Game) requirementsMet(gate store.Gate) bool {
	for _, r := range gate.Requires {
		if !g.K.Knows(r) {
			return false
		}
	}
	return true
}

// SceneView собирает срез сцены для правил. Здесь нет и не может быть графа
// фактов и truth: типа SceneView для них просто нет полей.
func (g *Game) SceneView(in Intent) SceneView {
	ch := g.DB.Characters[g.Actor]
	view := SceneView{
		Node:       g.Node,
		NodeTags:   g.nodeTags(g.Node),
		Allies:     1,
		Foes:       g.hostileCount(),
		Undetected: !g.Detected,
		ActorTier:  1,
		TargetTier: 1,
	}
	if ch != nil {
		view.Harm, view.Grit, view.Sheet = ch.Harm, ch.Grit, ch.Sheet
	}
	if h, ok := g.holderFor(in); ok {
		view.GateThreshold = h.Gate.Threshold
	}
	return view
}

func (g *Game) nodeTags(n store.NodeID) []string {
	return g.DB.Locations[n].Adjacent[:0:0] // теги приходят из дела в Task 15
}

func (g *Game) hostileCount() int {
	n := 0
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if g.Disposition[e.ID] <= -2 {
			n++
		}
	}
	return n
}

func (g *Game) flavourKey(in Intent) string {
	if in.Args.Topic != "" {
		return string(in.Verb) + "." + string(in.Args.Target) + "." + string(in.Args.Topic)
	}
	if in.Args.Target != "" {
		return string(in.Verb) + "." + string(in.Args.Target)
	}
	return string(in.Verb) + "." + string(g.Node)
}

func refuse(msg string) TurnResult { return TurnResult{Refused: true, Refusal: msg} }
```

- [ ] **Step 4: Прогнать тест, убедиться что проходит**

Run: `go test ./core/ -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add core/turn.go core/turn_test.go
git commit -m "feat(core): пятишаговый ход с отказами, mandatory-веткой и применением исхода"
```

---

### Task 14: Противоречия и `compare` без броска

**Files:**
- Modify: `store/rows.go` — добавить тип `Contradiction`; `store/db.go` — добавить поле `Contradictions []Contradiction`; `store/rows.go` — добавить `Tags []string` в `Location`
- Modify: `core/turn.go` — заменить временную реализацию `nodeTags`
- Create: `core/compare.go`
- Test: `core/compare_test.go`

**Interfaces:**
- Consumes: Tasks 2, 7, 13
- Produces: `store.Contradiction{A, B store.FactID, Reveals store.FactID, FlavourKey string}`, `store.DB.Contradictions`, `store.Location.Tags`, `(*Game).Compare(a, b store.FactID) TurnResult`

- [ ] **Step 1: Написать падающий тест**

`core/compare_test.go`:

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func compareGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Tags: []string{"dark", "rain"}}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	for _, id := range []store.FactID{"f_alibi", "f_seen_at_quay", "f_lie", "f_unrelated"} {
		db.Facts[id] = store.Fact{ID: id, Key: string(id)}
	}
	db.Contradictions = append(db.Contradictions, store.Contradiction{
		A: "f_alibi", B: "f_seen_at_quay", Reveals: "f_lie", FlavourKey: "compare.alibi_quay",
	})
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth: accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{"compare.alibi_quay": "Одно из двух — ложь."},
		Start: "n_quay", Actor: "pc",
	})
}

func TestCompareNeedsBothFactsKnown(t *testing.T) {
	g := compareGame()
	g.K.Learn("f_alibi", "e_ivar")
	got := g.Compare("f_alibi", "f_seen_at_quay")
	if !got.Refused {
		t.Error("сопоставление прошло с одним известным фактом из двух")
	}
}

func TestCompareFindsContradictionWithoutRoll(t *testing.T) {
	g := compareGame()
	g.K.Learn("f_alibi", "e_ivar")
	g.K.Learn("f_seen_at_quay", "e_nils")
	got := g.Compare("f_alibi", "f_seen_at_quay")
	if got.Refused {
		t.Fatalf("сопоставление отвергнуто: %s", got.Refusal)
	}
	if got.Res != nil {
		t.Error("compare бросил кость — противоречие это свойство данных, не удача")
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_lie" {
		t.Fatalf("противоречие не открыло факт: %v", got.Learned)
	}
	if got.FlavourKey != "compare.alibi_quay" {
		t.Errorf("ключ флейвора = %q", got.FlavourKey)
	}
}

func TestCompareIsOrderIndependent(t *testing.T) {
	g := compareGame()
	g.K.Learn("f_alibi", "e_ivar")
	g.K.Learn("f_seen_at_quay", "e_nils")
	got := g.Compare("f_seen_at_quay", "f_alibi") // обратный порядок
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_lie" {
		t.Errorf("порядок аргументов изменил результат: %v", got.Learned)
	}
}

func TestCompareOfUnrelatedFactsCostsNothing(t *testing.T) {
	g := compareGame()
	g.K.Learn("f_alibi", "e_ivar")
	g.K.Learn("f_unrelated", "e_bern")
	got := g.Compare("f_alibi", "f_unrelated")
	if got.Refused {
		t.Fatal("сопоставление известных фактов отвергнуто")
	}
	if len(got.Learned) != 0 {
		t.Errorf("несвязанные факты дали открытие: %v", got.Learned)
	}
	if len(got.Costs) != 0 {
		t.Errorf("пустое сопоставление стоило %v", got.Costs)
	}
}

func TestNodeTagsComeFromCase(t *testing.T) {
	g := compareGame()
	view := g.SceneView(Intent{Verb: "look"})
	if !view.HasTag("dark") || !view.HasTag("rain") {
		t.Errorf("теги локации не доехали до SceneView: %v", view.NodeTags)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run 'TestCompare|TestNodeTags' -v`
Expected: FAIL — `unknown field Tags`, `undefined: Compare`

- [ ] **Step 3: Расширить `store`**

В `store/rows.go` добавить поле в `Location`:

```go
type Location struct {
	ID       NodeID   `json:"id"`
	Name     string   `json:"name"`
	Adjacent []NodeID `json:"adjacent"`
	Tags     []string `json:"tags"` // dark, rain, crowd, indoors — читаются правилами
}
```

И новый тип в конец `store/rows.go`:

```go
// Contradiction — пара фактов, чьё сопоставление открывает третий факт.
// Отношение симметрично: порядок аргументов compare роли не играет.
type Contradiction struct {
	A          FactID `json:"a"`
	B          FactID `json:"b"`
	Reveals    FactID `json:"reveals"`
	FlavourKey string `json:"flavour_key"`
}
```

В `store/db.go` добавить поле в `DB`:

```go
	Contradictions []Contradiction
```

- [ ] **Step 4: Заменить временный `nodeTags` в `core/turn.go`**

Заменить:

```go
func (g *Game) nodeTags(n store.NodeID) []string {
	return g.DB.Locations[n].Adjacent[:0:0] // теги приходят из дела в Task 15
}
```

на:

```go
func (g *Game) nodeTags(n store.NodeID) []string {
	return g.DB.Locations[n].Tags
}
```

- [ ] **Step 5: Написать `core/compare.go`**

```go
package core

import "github.com/kliuchnikovv/dnd/store"

// Compare сопоставляет два известных факта. Броска здесь нет и не будет:
// противоречие между двумя фактами либо есть, либо нет — это свойство данных.
// Игрок сопоставил — игрок заметил.
func (g *Game) Compare(a, b store.FactID) TurnResult {
	if a == b {
		return refuse("сопоставлять факт с самим собой нечего")
	}
	if !g.K.Knows(a) || !g.K.Knows(b) {
		return refuse("оба факта должны быть известны парти")
	}
	for _, c := range g.DB.Contradictions {
		if (c.A == a && c.B == b) || (c.A == b && c.B == a) {
			out := TurnResult{FlavourKey: c.FlavourKey}
			// Вывод — не свидетельство: источником становится сама пара фактов,
			// поэтому запись идёт от служебной сущности рассуждения.
			if g.K.Learn(c.Reveals, ReasoningSource) {
				out.Learned = append(out.Learned, Learned{c.Reveals, ReasoningSource})
			}
			g.applyUnlocksFor(c.Reveals)
			return out
		}
	}
	return TurnResult{FlavourKey: "compare.nothing"}
}

// ReasoningSource — источник фактов, полученных рассуждением, а не
// свидетельством. Рёбер в relations у него нет, поэтому он не мешает
// корроборации и не считается за независимого свидетеля дважды.
const ReasoningSource store.EntityID = "e_reasoning"
```

- [ ] **Step 6: Добавить временную заглушку `applyUnlocksFor`**

Полная реализация — Task 15. В конец `core/compare.go`:

```go
// applyUnlocksFor заменяется полной реализацией в core/unlocks.go (Task 15).
func (g *Game) applyUnlocksFor(store.FactID) {}
```

- [ ] **Step 7: Прогнать тесты, убедиться что проходят**

Run: `go test ./core/ -v`
Expected: PASS

- [ ] **Step 8: Коммит**

```bash
git add store/ core/compare.go core/compare_test.go core/turn.go
git commit -m "feat(core): противоречия в данных и compare без броска"
```

---

### Task 15: Спящие факты и разблокировки

**Files:**
- Create: `core/unlocks.go`
- Modify: `core/compare.go` — удалить заглушку `applyUnlocksFor`; `core/turn.go` — вызвать `applyUnlocksFor` после каждого выученного факта и учесть `latent` в `holderFor`
- Test: `core/unlocks_test.go`

**Interfaces:**
- Consumes: Tasks 7, 13, 14
- Produces: `(*Game).applyUnlocksFor(store.FactID)`, `(*Game).Unlocked(kind, id string) bool`, `(*Game).ReachableNodes() []store.NodeID`

- [ ] **Step 1: Написать падающий тест**

`core/unlocks_test.go`:

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func unlockGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Adjacent: []store.NodeID{"n_cellar"}}
	db.Locations["n_cellar"] = store.Location{ID: "n_cellar", Adjacent: []store.NodeID{"n_quay"}}
	db.Entities["e_toke"] = store.Entity{ID: "e_toke", Kind: store.EntityNPC, Node: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_key"] = store.Fact{ID: "f_key"}
	db.Facts["f_deep"] = store.Fact{ID: "f_deep"}
	// Спящий держатель: факт у Токе есть, но темы нет, пока не открыт f_key.
	db.Holders["f_deep"] = []store.FactHolder{{
		FactID: "f_deep", HolderID: "e_toke", Mandatory: true, Latent: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}
	db.Unlocks["f_key"] = []store.FactUnlock{
		{FactID: "f_key", UnlocksKind: "topic", UnlocksID: "f_deep"},
		{FactID: "f_key", UnlocksKind: "node", UnlocksID: "n_cellar"},
	}
	db.Clocks["c"] = &store.Clock{ID: "c", Segments: 6, TickPolicy: "on_cost"}
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth: accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func TestLatentHolderIsInvisibleUntilUnlocked(t *testing.T) {
	g := unlockGame()
	g.K.Learn("f_deep", "e_bern") // тема формально в банке
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_deep"}})
	if !got.Refused {
		t.Error("спящий держатель отдал факт до разблокировки")
	}
}

func TestLearningUnlocksTopicAndNode(t *testing.T) {
	g := unlockGame()
	if g.Unlocked("node", "n_cellar") {
		t.Fatal("узел разблокирован до открытия факта")
	}
	g.K.Learn("f_key", "e_toke")
	g.applyUnlocksFor("f_key")
	if !g.Unlocked("topic", "f_deep") {
		t.Error("тема не разблокирована")
	}
	if !g.Unlocked("node", "n_cellar") {
		t.Error("узел не разблокирован")
	}
	g.K.Learn("f_deep", "e_bern")
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_deep"}})
	if got.Refused {
		t.Errorf("спящий держатель молчит после разблокировки: %s", got.Refusal)
	}
}

func TestUnlocksFireFromApply(t *testing.T) {
	// Разблокировка должна происходить сама, а не по отдельному вызову.
	g := unlockGame()
	g.DB.Holders["f_key"] = []store.FactHolder{{
		FactID: "f_key", HolderID: "e_toke", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}
	g.K.Learn("f_key", "e_bern") // тема в банке
	g.Apply(Intent{Verb: "question", Args: Args{Target: "e_toke", Topic: "f_key"}})
	if !g.Unlocked("node", "n_cellar") {
		t.Error("Apply не применил разблокировки выученного факта")
	}
}

func TestReachableNodesRespectLocks(t *testing.T) {
	g := unlockGame()
	if got := g.ReachableNodes(); len(got) != 0 {
		t.Errorf("заблокированный узел объявлен достижимым: %v", got)
	}
	g.K.Learn("f_key", "e_toke")
	g.applyUnlocksFor("f_key")
	got := g.ReachableNodes()
	if len(got) != 1 || got[0] != "n_cellar" {
		t.Errorf("достижимые узлы = %v, ожидалось [n_cellar]", got)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run 'TestLatent|TestLearningUnlocks|TestUnlocksFire|TestReachable' -v`
Expected: FAIL — `undefined: Unlocked`

- [ ] **Step 3: Удалить заглушку из `core/compare.go`**

Удалить строки:

```go
// applyUnlocksFor заменяется полной реализацией в core/unlocks.go (Task 15).
func (g *Game) applyUnlocksFor(store.FactID) {}
```

- [ ] **Step 4: Написать `core/unlocks.go`**

```go
package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

// applyUnlocksFor открывает то, что было заперто выученным фактом. Это и
// превращает набор проверок в расследование: узнал одно — появилось, о чём
// спрашивать и куда идти.
func (g *Game) applyUnlocksFor(f store.FactID) {
	if g.unlocked == nil {
		g.unlocked = map[string]bool{}
	}
	for _, u := range g.DB.Unlocks[f] {
		g.unlocked[u.UnlocksKind+":"+u.UnlocksID] = true
	}
}

func (g *Game) Unlocked(kind, id string) bool { return g.unlocked[kind+":"+id] }

// ReachableNodes — смежные узлы, открытые к посещению. Узел, не упомянутый ни
// в одном fact_unlocks, считается открытым изначально.
func (g *Game) ReachableNodes() []store.NodeID {
	var out []store.NodeID
	for _, n := range g.DB.Locations[g.Node].Adjacent {
		if g.nodeLocked(n) && !g.Unlocked("node", string(n)) {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (g *Game) nodeLocked(n store.NodeID) bool {
	for _, us := range g.DB.Unlocks {
		for _, u := range us {
			if u.UnlocksKind == "node" && u.UnlocksID == string(n) {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 5: Добавить поле `unlocked` в `core/game.go`**

В структуру `Game` добавить поле, в `NewGame` — его инициализацию:

```go
	unlocked map[string]bool
```

```go
		unlocked:    map[string]bool{},
```

- [ ] **Step 6: Учесть `latent` и разблокировки в `core/turn.go`**

В `holderFor` после проверки `requirementsMet` добавить:

```go
		if h.Latent && !g.Unlocked("topic", string(h.FactID)) {
			continue
		}
```

В `Apply` — вызвать разблокировки после каждой записи факта. В ветке без броска и в ветке применения исхода заменить блок записи на:

```go
		if g.K.Learn(holder.FactID, holder.HolderID) {
			res.Learned = append(res.Learned, Learned{holder.FactID, holder.HolderID})
			g.applyUnlocksFor(holder.FactID)
		}
```

(во второй ветке переменная называется `out`, а не `res` — заменить соответственно)

Также в `validate` заменить проверку прохода по узлу, чтобы запертый узел давал отказ:

```go
	if in.Args.Node != "" {
		if !g.DB.Adjacent(g.Node, in.Args.Node) {
			return refuse("туда отсюда не пройти"), true
		}
		if g.nodeLocked(in.Args.Node) && !g.Unlocked("node", string(in.Args.Node)) {
			return refuse("туда пока незачем идти"), true
		}
	}
```

- [ ] **Step 7: Прогнать тесты, убедиться что проходят**

Run: `go test ./core/ -v`
Expected: PASS

- [ ] **Step 8: Коммит**

```bash
git add core/unlocks.go core/unlocks_test.go core/game.go core/turn.go core/compare.go
git commit -m "feat(core): спящие держатели и разблокировки тем и узлов"
```

---

### Task 16: Форма обвинения — токены, попытки, цена

**Files:**
- Create: `core/accuse.go`
- Test: `core/accuse_test.go`

**Interfaces:**
- Consumes: Tasks 6, 7, 8, 12
- Produces: `(*Game).AvailableTokens(slot string) []store.Token`, `(*Game).Accuse(accusation.Form) AccusationResult`, `core.AccusationResult{Refused bool, Refusal string, Correct bool, Attempt int, Fired []store.Consequence}`

- [ ] **Step 1: Написать падающий тест**

`core/accuse_test.go`:

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func accuseGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Clocks["c_suspicion"] = &store.Clock{ID: "c_suspicion", Segments: 6, TickPolicy: "on_cost"}
	for _, f := range []store.FactID{"f_who", "f_how", "f_when", "f_why", "f_wrong"} {
		db.Facts[f] = store.Fact{ID: f}
	}
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth: accusation.NewTruth("toke", "cord", "night", "audit"),
		Tokens: []TokenGrant{
			{Slot: "who", Token: "toke", Fact: "f_who"},
			{Slot: "who", Token: "ivar", Fact: "f_wrong"},
			{Slot: "how", Token: "cord", Fact: "f_how"},
			{Slot: "when", Token: "night", Fact: "f_when"},
			{Slot: "why", Token: "audit", Fact: "f_why"},
		},
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func learnAll(g *Game) {
	for _, f := range []store.FactID{"f_who", "f_how", "f_when", "f_why"} {
		g.K.Learn(f, "e_bern")
	}
}

func TestTokenAvailableOnlyUnderCollectedFact(t *testing.T) {
	g := accuseGame()
	if got := g.AvailableTokens("who"); len(got) != 0 {
		t.Fatalf("токены доступны без фактов: %v", got)
	}
	g.K.Learn("f_who", "e_bern")
	got := g.AvailableTokens("who")
	if len(got) != 1 || got[0] != "toke" {
		t.Errorf("доступные токены = %v, ожидалось [toke]", got)
	}
}

func TestAccusationWithUnavailableTokenIsRefused(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	form := accusation.Form{Who: "sigrid", How: "cord", When: "night", Why: "audit"}
	got := g.Accuse(form)
	if !got.Refused {
		t.Error("принят токен, не подкреплённый фактом")
	}
	if g.Attempts != 0 {
		t.Error("отклонённая форма засчитана попыткой")
	}
}

func TestCorrectAccusationSucceeds(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	got := g.Accuse(accusation.Form{Who: "toke", How: "cord", When: "night", Why: "audit"})
	if got.Refused {
		t.Fatalf("верное обвинение отвергнуто: %s", got.Refusal)
	}
	if !got.Correct {
		t.Error("верное обвинение признано ошибочным")
	}
	if got.Attempt != 1 {
		t.Errorf("номер попытки = %d, ожидался 1", got.Attempt)
	}
}

func TestEveryAttemptCostsAClockTick(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	g.K.Learn("f_wrong", "e_bern")
	before := g.DB.Clocks["c_suspicion"].Filled
	g.Accuse(accusation.Form{Who: "ivar", How: "cord", When: "night", Why: "audit"})
	if got := g.DB.Clocks["c_suspicion"].Filled; got != before+1 {
		t.Errorf("часы = %d, ожидалось %d — иначе слоты брутфорсятся", got, before+1)
	}
	if g.Attempts != 1 {
		t.Errorf("счётчик попыток = %d, ожидался 1", g.Attempts)
	}
}

func TestPartialMatchLeaksNothing(t *testing.T) {
	// Три верных слота и ноль верных должны быть неразличимы по результату.
	g := accuseGame()
	learnAll(g)
	g.K.Learn("f_wrong", "e_bern")

	three := g.Accuse(accusation.Form{Who: "ivar", How: "cord", When: "night", Why: "audit"})
	zero := g.Accuse(accusation.Form{Who: "ivar", How: "ivar", When: "ivar", Why: "ivar"})

	if three.Correct != zero.Correct {
		t.Fatal("частичное совпадение отличимо от полного промаха")
	}
	if three.Refusal != zero.Refusal {
		t.Errorf("тексты отказа различаются: %q против %q", three.Refusal, zero.Refusal)
	}
}

func TestIncompleteFormIsRefusedWithoutCost(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	before := g.DB.Clocks["c_suspicion"].Filled
	got := g.Accuse(accusation.Form{Who: "toke"})
	if !got.Refused {
		t.Error("неполная форма принята")
	}
	if g.DB.Clocks["c_suspicion"].Filled != before {
		t.Error("неполная форма стоила тика")
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./core/ -run 'TestToken|TestAccusation|TestCorrect|TestEveryAttempt|TestPartialMatch|TestIncomplete' -v`
Expected: FAIL — `undefined: AvailableTokens`

- [ ] **Step 3: Написать `core/accuse.go`**

```go
package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

type AccusationResult struct {
	Refused bool
	Refusal string
	Correct bool
	Attempt int
	Fired   []store.Consequence
}

// AvailableTokens возвращает токены слота, подкреплённые собранными фактами.
func (g *Game) AvailableTokens(slot string) []store.Token {
	var out []store.Token
	seen := map[store.Token]bool{}
	for _, t := range g.tokens {
		if t.Slot != slot || seen[t.Token] || !g.K.Knows(t.Fact) {
			continue
		}
		seen[t.Token] = true
		out = append(out, t.Token)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Accuse проверяет форму целиком. Ошибочная форма не сообщает, какой слот
// неверен, и стоит тика часов — иначе слоты брутфорсятся по одному.
func (g *Game) Accuse(f accusation.Form) AccusationResult {
	if !f.Complete() {
		return AccusationResult{Refused: true, Refusal: "форма заполнена не полностью"}
	}
	slots := map[string]store.Token{"who": f.Who, "how": f.How, "when": f.When, "why": f.Why}
	for slot, tok := range slots {
		if !g.tokenAvailable(slot, tok) {
			return AccusationResult{Refused: true,
				Refusal: "токен «" + string(tok) + "» не подкреплён собранным фактом"}
		}
	}

	g.Attempts++
	res := AccusationResult{Attempt: g.Attempts, Correct: g.truth.Check(f)}
	fired := g.C.TickAll(1)
	g.applyConsequences(fired)
	res.Fired = fired
	return res
}

func (g *Game) tokenAvailable(slot string, tok store.Token) bool {
	for _, t := range g.AvailableTokens(slot) {
		if t == tok {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Прогнать тесты, убедиться что проходят**

Run: `go test ./core/ -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add core/accuse.go core/accuse_test.go
git commit -m "feat(core): форма обвинения с токенами под факты, счётчиком и ценой попытки"
```

---

### Task 17: Формат дела и загрузчик

**Files:**
- Create: `cases/schema.go`, `cases/load.go`
- Test: `cases/load_test.go`, `cases/testdata/minimal.json`

**Interfaces:**
- Consumes: `store`, `core`, `core/accusation`
- Produces: `cases.File` (JSON-форма дела), `cases.Load(path string) (*core.Config, error)`, `cases.Parse([]byte) (*core.Config, error)`

- [ ] **Step 1: Написать тестовые данные**

`cases/testdata/minimal.json` — минимальное валидное дело, на котором проверяется загрузчик:

```json
{
  "id": "minimal",
  "archetype": "murder",
  "start": "n_quay",
  "actor": "pc",
  "character": {
    "id": "pc",
    "grit": 3,
    "sheet": {
      "archetype": "inspector",
      "attrs": {"body": 0, "edge": 1, "mind": 3, "will": 0},
      "tags": [{"name": "портовый", "verbs": ["question", "search"]}],
      "abilities": ["read_room"]
    }
  },
  "locations": [
    {"id": "n_quay", "name": "Пристань", "adjacent": ["n_forge"], "tags": ["rain"]},
    {"id": "n_forge", "name": "Кузница", "adjacent": ["n_quay"], "tags": ["indoors"]}
  ],
  "entities": [
    {"id": "e_toke", "name": "Токе", "kind": "npc", "voice": "сухой", "node": "n_quay"},
    {"id": "e_body", "name": "Тело Халдена", "kind": "thing", "node": "n_quay"}
  ],
  "relations": [],
  "facts": [
    {"id": "f_ligature", "key": "След шнура на шее", "kind": "concept"}
  ],
  "fact_holders": [
    {"fact_id": "f_ligature", "holder_id": "e_body", "mandatory": true, "latent": false,
     "gate": {"verbs": ["examine"], "threshold": "normal", "requires": []}}
  ],
  "fact_unlocks": [],
  "contradictions": [],
  "clocks": [
    {"id": "c_suspicion", "name": "Подозрение", "segments": 6, "tick_policy": "on_cost",
     "on_fill": {"flavour_key": "clock.suspicion.filled", "hostile_to": ["e_toke"]}}
  ],
  "accusation_tokens": [
    {"slot": "who", "token": "toke", "fact": "f_ligature"},
    {"slot": "how", "token": "cord", "fact": "f_ligature"},
    {"slot": "when", "token": "night", "fact": "f_ligature"},
    {"slot": "why", "token": "audit", "fact": "f_ligature"}
  ],
  "truth": {"who": "toke", "how": "cord", "when": "night", "why": "audit"},
  "start_facts": [{"fact": "f_ligature", "from": "e_body"}],
  "flavour": {
    "look.n_quay": "Дождь бьёт по доскам пристани.",
    "examine.e_body.f_ligature": "На шее — узкая борозда.",
    "clock.suspicion.filled": "Город закрылся."
  }
}
```

- [ ] **Step 2: Написать падающий тест**

`cases/load_test.go`:

```go
package cases

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadBuildsPlayableConfig(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка: %v", err)
	}
	if cfg.Start != "n_quay" || cfg.Actor != "pc" {
		t.Errorf("стартовое состояние: узел %q, актор %q", cfg.Start, cfg.Actor)
	}
	if len(cfg.DB.Locations) != 2 || len(cfg.DB.Entities) != 2 {
		t.Errorf("таблицы заполнены не полностью: %d локаций, %d сущностей",
			len(cfg.DB.Locations), len(cfg.DB.Entities))
	}
	if got := cfg.DB.Locations["n_quay"].Tags; len(got) != 1 || got[0] != "rain" {
		t.Errorf("теги локации = %v", got)
	}
	if len(cfg.DB.Holders["f_ligature"]) != 1 {
		t.Error("держатель факта не загружен")
	}
	if cfg.Flavour["look.n_quay"] == "" {
		t.Error("флейвор не загружен")
	}
	if len(cfg.Tokens) != 4 {
		t.Errorf("токенов обвинения %d, ожидалось 4", len(cfg.Tokens))
	}
}

func TestStartFactsArePrewritten(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DB.Knowledge) != 1 || cfg.DB.Knowledge[0].FactID != "f_ligature" {
		t.Errorf("стартовые факты не записаны: %v", cfg.DB.Knowledge)
	}
}

func TestSheetSurvivesAsRawBytes(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Attrs map[string]int `json:"attrs"`
	}
	if err := json.Unmarshal(cfg.DB.Characters["pc"].Sheet, &probe); err != nil {
		t.Fatalf("лист не разобрался: %v", err)
	}
	if probe.Attrs["mind"] != 3 {
		t.Errorf("mind = %d, ожидалось 3", probe.Attrs["mind"])
	}
}

func TestTruthNeverAppearsInConfigDump(t *testing.T) {
	cfg, err := Load("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	// Сериализация конфига обязана падать, а не печатать правильный ответ.
	if b, err := json.Marshal(cfg.Truth); err == nil {
		t.Fatalf("truth сериализовался: %s", b)
	}
	if s := cfg.Truth.String(); strings.Contains(s, "toke") {
		t.Errorf("truth просочился: %q", s)
	}
}

func TestBrokenJSONReportsPath(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.json"); err == nil {
		t.Fatal("несуществующий файл загрузился")
	}
	if _, err := Parse([]byte("{не json")); err == nil {
		t.Fatal("битый JSON загрузился")
	}
}
```

- [ ] **Step 3: Прогнать тест, убедиться что падает**

Run: `go test ./cases/ -v`
Expected: FAIL — `undefined: Load`

- [ ] **Step 4: Написать `cases/schema.go`**

```go
// Package cases загружает рукописное дело из JSON и проверяет его инварианты.
package cases

import (
	"encoding/json"

	"github.com/kliuchnikovv/dnd/store"
)

// File — JSON-форма дела. Поля повторяют таблицы: файл читается как дамп базы,
// а не как отдельный формат со своей логикой.
type File struct {
	ID        store.CaseID `json:"id"`
	Archetype string       `json:"archetype"`
	Start     store.NodeID `json:"start"`
	Actor     string       `json:"actor"`

	Character struct {
		ID    string          `json:"id"`
		Grit  int             `json:"grit"`
		Harm  int             `json:"harm"`
		Sheet json.RawMessage `json:"sheet"`
	} `json:"character"`

	Locations      []store.Location      `json:"locations"`
	Entities       []store.Entity        `json:"entities"`
	Relations      []store.Relation      `json:"relations"`
	Facts          []store.Fact          `json:"facts"`
	FactHolders    []store.FactHolder    `json:"fact_holders"`
	FactUnlocks    []store.FactUnlock    `json:"fact_unlocks"`
	Contradictions []store.Contradiction `json:"contradictions"`
	Clocks         []store.Clock         `json:"clocks"`

	Tokens []struct {
		Slot  string       `json:"slot"`
		Token store.Token  `json:"token"`
		Fact  store.FactID `json:"fact"`
	} `json:"accusation_tokens"`

	Truth struct {
		Who  store.Token `json:"who"`
		How  store.Token `json:"how"`
		When store.Token `json:"when"`
		Why  store.Token `json:"why"`
	} `json:"truth"`

	StartFacts []struct {
		Fact store.FactID   `json:"fact"`
		From store.EntityID `json:"from"`
	} `json:"start_facts"`

	Flavour map[string]string `json:"flavour"`
}
```

- [ ] **Step 5: Написать `cases/load.go`**

```go
package cases

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func Load(path string) (*core.Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("чтение дела %s: %w", path, err)
	}
	cfg, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("разбор дела %s: %w", path, err)
	}
	return cfg, nil
}

func Parse(raw []byte) (*core.Config, error) {
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}

	db := store.NewDB()
	for _, l := range f.Locations {
		db.Locations[l.ID] = l
	}
	for _, e := range f.Entities {
		db.Entities[e.ID] = e
	}
	db.Relations = append(db.Relations, f.Relations...)
	for _, fact := range f.Facts {
		fact.CaseID = f.ID
		db.Facts[fact.ID] = fact
	}
	for _, h := range f.FactHolders {
		db.Holders[h.FactID] = append(db.Holders[h.FactID], h)
	}
	for _, u := range f.FactUnlocks {
		db.Unlocks[u.FactID] = append(db.Unlocks[u.FactID], u)
	}
	db.Contradictions = append(db.Contradictions, f.Contradictions...)
	for i := range f.Clocks {
		c := f.Clocks[i]
		db.Clocks[c.ID] = &c
	}
	db.Characters[store.CharacterID(f.Character.ID)] = &store.Character{
		ID:    store.CharacterID(f.Character.ID),
		Sheet: f.Character.Sheet,
		Grit:  f.Character.Grit,
		Harm:  f.Character.Harm,
	}
	// Стартовые факты пишутся прямо в party_knowledge: расследование начинается
	// не с пустого листа, иначе первый ход некуда сделать.
	for i, s := range f.StartFacts {
		db.Knowledge = append(db.Knowledge, store.Knowledge{
			FactID: s.Fact, LearnedFrom: s.From,
			Confidence: core.SourceConfidence, LearnedAt: i + 1,
		})
	}

	var tokens []core.TokenGrant
	for _, t := range f.Tokens {
		tokens = append(tokens, core.TokenGrant{Slot: t.Slot, Token: t.Token, Fact: t.Fact})
	}

	return &core.Config{
		DB:      db,
		Truth:   accusation.NewTruth(f.Truth.Who, f.Truth.How, f.Truth.When, f.Truth.Why),
		Flavour: f.Flavour,
		Tokens:  tokens,
		Start:   f.Start,
		Actor:   store.CharacterID(f.Actor),
	}, nil
}
```

- [ ] **Step 6: Прогнать тесты, убедиться что проходят**

Run: `go test ./cases/ -v`
Expected: PASS

- [ ] **Step 7: Коммит**

```bash
git add cases/
git commit -m "feat(cases): JSON-форма дела и загрузчик в таблицы ядра"
```

---

### Task 18: Валидатор инвариантов дела

**Files:**
- Create: `cases/validate.go`
- Modify: `cases/load.go` — вызвать `Validate` в конце `Parse`
- Test: `cases/validate_test.go`

**Interfaces:**
- Consumes: Task 17
- Produces: `validateFile(File) error` — не экспортируется, вызывается из `Parse`; возвращает все нарушения разом, а не первое

- [ ] **Step 1: Написать падающий тест**

`cases/validate_test.go`:

```go
package cases

import (
	"os"
	"strings"
	"testing"
)

func mutate(t *testing.T, edit func(*File)) error {
	t.Helper()
	raw, err := os.ReadFile("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	f := decodeFile(t, raw)
	edit(&f)
	return validateFile(f)
}

func TestMinimalCaseIsValid(t *testing.T) {
	if _, err := Load("testdata/minimal.json"); err != nil {
		t.Fatalf("эталонное дело не проходит валидацию: %v", err)
	}
}

func TestFactWithoutMandatoryHolderIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].Mandatory = false })
	if err == nil || !strings.Contains(err.Error(), "mandatory") {
		t.Fatalf("факт без mandatory-держателя принят: %v", err)
	}
}

func TestDanglingHolderReferenceIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].HolderID = "e_nobody" })
	if err == nil || !strings.Contains(err.Error(), "e_nobody") {
		t.Fatalf("держатель-призрак принят: %v", err)
	}
}

func TestAsymmetricAdjacencyIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Locations[1].Adjacent = nil })
	if err == nil || !strings.Contains(err.Error(), "смежност") {
		t.Fatalf("односторонний проход принят: %v", err)
	}
}

func TestMissingFlavourKeyIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { delete(f.Flavour, "clock.suspicion.filled") })
	if err == nil || !strings.Contains(err.Error(), "clock.suspicion.filled") {
		t.Fatalf("отсутствующий ключ флейвора принят: %v", err)
	}
}

func TestTruthSlotWithoutTokenIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Truth.Why = "нет_такого_токена" })
	if err == nil || !strings.Contains(err.Error(), "why") {
		t.Fatalf("правильный ответ, недостижимый ни одним токеном, принят: %v", err)
	}
}

func TestUnknownVerbInGateIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].Gate.Verbs = []string{"hack"} })
	if err == nil || !strings.Contains(err.Error(), "hack") {
		t.Fatalf("несуществующий глагол в gate принят: %v", err)
	}
}

func TestValidatorReportsEveryViolationAtOnce(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.FactHolders[0].Mandatory = false
		f.FactHolders[0].HolderID = "e_nobody"
	})
	if err == nil {
		t.Fatal("нарушения не обнаружены")
	}
	if !strings.Contains(err.Error(), "mandatory") || !strings.Contains(err.Error(), "e_nobody") {
		t.Errorf("сообщено не всё: %v", err)
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./cases/ -run TestValidat -v`
Expected: FAIL — `undefined: decodeFile`

- [ ] **Step 3: Написать `cases/validate.go`**

```go
package cases

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// validateFile проверяет инварианты рукописного дела. Падает при загрузке, а не
// на сороковой минуте прогона. Возвращает все нарушения разом: чинить дело по
// одному сообщению за запуск — это часы вместо минут.
func validateFile(f File) error {
	var bad []string
	add := func(format string, args ...any) { bad = append(bad, fmt.Sprintf(format, args...)) }

	entities := map[store.EntityID]bool{}
	for _, e := range f.Entities {
		entities[e.ID] = true
		if e.Node != "" && !hasLocation(f, e.Node) {
			add("сущность %s стоит в несуществующем узле %s", e.ID, e.Node)
		}
	}
	facts := map[store.FactID]bool{}
	for _, fact := range f.Facts {
		facts[fact.ID] = true
	}

	// Смежность симметрична: проход в одну сторону — почти всегда опечатка.
	for _, l := range f.Locations {
		for _, n := range l.Adjacent {
			if !hasLocation(f, n) {
				add("узел %s ведёт в несуществующий %s", l.ID, n)
				continue
			}
			if !adjacent(f, n, l.ID) {
				add("нарушена смежность: %s -> %s есть, обратного нет", l.ID, n)
			}
		}
	}

	mandatory := map[store.FactID]bool{}
	for _, h := range f.FactHolders {
		if !facts[h.FactID] {
			add("держатель ссылается на несуществующий факт %s", h.FactID)
		}
		if !entities[h.HolderID] {
			add("факт %s держит несуществующая сущность %s", h.FactID, h.HolderID)
		}
		if len(h.Gate.Verbs) == 0 {
			add("у держателя факта %s пустой список глаголов", h.FactID)
		}
		for _, v := range h.Gate.Verbs {
			if _, ok := core.LookupVerb(v); !ok {
				add("gate факта %s ссылается на несуществующий глагол %q", h.FactID, v)
			}
		}
		for _, r := range h.Gate.Requires {
			if !facts[r] {
				add("gate факта %s требует несуществующий факт %s", h.FactID, r)
			}
		}
		if h.Mandatory {
			mandatory[h.FactID] = true
		}
	}

	// Единственная защита от «кубик убил дело».
	for _, fact := range f.Facts {
		if !mandatory[fact.ID] && !revealedByCompare(f, fact.ID) {
			add("у факта %s нет ни одного mandatory-держателя", fact.ID)
		}
	}

	// Каждый слот правильного ответа должен быть достижим токеном под фактом.
	tokens := map[string]map[store.Token]bool{}
	for _, t := range f.Tokens {
		if !facts[t.Fact] {
			add("токен %s опирается на несуществующий факт %s", t.Token, t.Fact)
		}
		if tokens[t.Slot] == nil {
			tokens[t.Slot] = map[store.Token]bool{}
		}
		tokens[t.Slot][t.Token] = true
	}
	for slot, want := range map[string]store.Token{
		"who": f.Truth.Who, "how": f.Truth.How, "when": f.Truth.When, "why": f.Truth.Why,
	} {
		if !tokens[slot][want] {
			add("слот %s правильного ответа не покрыт ни одним токеном", slot)
		}
	}

	for _, c := range f.Contradictions {
		for _, id := range []store.FactID{c.A, c.B, c.Reveals} {
			if !facts[id] {
				add("противоречие ссылается на несуществующий факт %s", id)
			}
		}
		if f.Flavour[c.FlavourKey] == "" {
			add("у противоречия нет текста по ключу %s", c.FlavourKey)
		}
	}

	for _, cl := range f.Clocks {
		if cl.Segments <= 0 {
			add("у часов %s неположительное число сегментов", cl.ID)
		}
		if cl.OnFill.FlavourKey != "" && f.Flavour[cl.OnFill.FlavourKey] == "" {
			add("у часов %s нет текста по ключу %s", cl.ID, cl.OnFill.FlavourKey)
		}
	}

	for _, u := range f.FactUnlocks {
		if !facts[u.FactID] {
			add("разблокировка исходит из несуществующего факта %s", u.FactID)
		}
		switch u.UnlocksKind {
		case "topic":
			if !facts[store.FactID(u.UnlocksID)] {
				add("разблокировка открывает несуществующую тему %s", u.UnlocksID)
			}
		case "node":
			if !hasLocation(f, store.NodeID(u.UnlocksID)) {
				add("разблокировка открывает несуществующий узел %s", u.UnlocksID)
			}
		case "entity":
			if !entities[store.EntityID(u.UnlocksID)] {
				add("разблокировка открывает несуществующую сущность %s", u.UnlocksID)
			}
		default:
			add("неизвестный вид разблокировки %q", u.UnlocksKind)
		}
	}

	if len(f.StartFacts) == 0 {
		add("у дела нет стартовых фактов — первый ход некуда сделать")
	}

	if len(bad) > 0 {
		return errors.New("дело не прошло валидацию:\n  - " + strings.Join(bad, "\n  - "))
	}
	return nil
}

func hasLocation(f File, id store.NodeID) bool {
	for _, l := range f.Locations {
		if l.ID == id {
			return true
		}
	}
	return false
}

func adjacent(f File, from, to store.NodeID) bool {
	for _, l := range f.Locations {
		if l.ID != from {
			continue
		}
		for _, n := range l.Adjacent {
			if n == to {
				return true
			}
		}
	}
	return false
}

// revealedByCompare — факт-вывод не нуждается в держателе: его открывает
// сопоставление, а оно броска не требует и потому кубиком не блокируется.
func revealedByCompare(f File, id store.FactID) bool {
	for _, c := range f.Contradictions {
		if c.Reveals == id {
			return true
		}
	}
	for _, s := range f.StartFacts {
		if s.Fact == id {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Дописать вспомогательную функцию в тест**

`decodeFile` живёт в тестовом файле, а не в `validate.go`: импорт `testing` в обычном файле тянет тестовый пакет в бинарь. Добавить в начало `cases/validate_test.go` (и `"encoding/json"` в его импорты):

```go
func decodeFile(t *testing.T, raw []byte) File {
	t.Helper()
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("разбор эталонного дела: %v", err)
	}
	return f
}
```

- [ ] **Step 5: Вызвать валидатор из `Parse`**

В `cases/load.go`, в конце `Parse`, перед `return`:

```go
	if err := validateFile(f); err != nil {
		return nil, err
	}
```

- [ ] **Step 6: Прогнать тесты, убедиться что проходят**

Run: `go test ./cases/ -v`
Expected: PASS

- [ ] **Step 7: Коммит**

```bash
git add cases/validate.go cases/validate_test.go cases/load.go
git commit -m "feat(cases): валидатор инвариантов дела, сообщающий все нарушения разом"
```

---

### Task 19: Парсер команд

**Files:**
- Create: `cli/parse.go`
- Test: `cli/parse_test.go`

**Interfaces:**
- Consumes: `core.Intent`, `core.Verbs`
- Produces: `cli.Command{Kind CommandKind, Intent core.Intent, Facts []store.FactID, Text string}`, `cli.CommandKind` с `CmdAction|CmdCompare|CmdAccuse|CmdFacts|CmdState|CmdClocks|CmdHelp|CmdQuit`, `cli.Parse(line string) (Command, error)`

- [ ] **Step 1: Написать падающий тест**

`cli/parse_test.go`:

```go
package cli

import "testing"

func TestParseStructuredAction(t *testing.T) {
	cmd, err := Parse("question ivar ledger")
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if cmd.Kind != CmdAction {
		t.Fatalf("вид команды %v", cmd.Kind)
	}
	if cmd.Intent.Verb != "question" {
		t.Errorf("глагол %q", cmd.Intent.Verb)
	}
	if cmd.Intent.Args.Target != "e_ivar" {
		t.Errorf("цель %q, ожидалось e_ivar", cmd.Intent.Args.Target)
	}
	if cmd.Intent.Args.Topic != "f_ledger" {
		t.Errorf("тема %q, ожидалось f_ledger", cmd.Intent.Args.Topic)
	}
}

func TestParseAcceptsFullIdentifiers(t *testing.T) {
	// Игрок может ввести и короткое имя, и полный id из вывода facts.
	cmd, _ := Parse("question e_ivar f_ledger")
	if cmd.Intent.Args.Target != "e_ivar" || cmd.Intent.Args.Topic != "f_ledger" {
		t.Errorf("полные идентификаторы искажены: %+v", cmd.Intent.Args)
	}
}

func TestParseSingleArgVerbs(t *testing.T) {
	cases := map[string]struct{ verb, target string }{
		"examine body":      {"examine", "e_body"},
		"search warehouse":  {"search", "e_warehouse"},
		"stake_out quay":    {"stake_out", "e_quay"},
	}
	for line, want := range cases {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if string(cmd.Intent.Verb) != want.verb || string(cmd.Intent.Args.Target) != want.target {
			t.Errorf("%q -> %q %q", line, cmd.Intent.Verb, cmd.Intent.Args.Target)
		}
	}
}

func TestParseCompareTakesTwoFacts(t *testing.T) {
	cmd, err := Parse("compare alibi seen_at_quay")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Kind != CmdCompare {
		t.Fatalf("вид команды %v", cmd.Kind)
	}
	if len(cmd.Facts) != 2 || cmd.Facts[0] != "f_alibi" || cmd.Facts[1] != "f_seen_at_quay" {
		t.Errorf("факты = %v", cmd.Facts)
	}
}

func TestParseBareCommands(t *testing.T) {
	cases := map[string]CommandKind{
		"facts": CmdFacts, "state": CmdState, "clocks": CmdClocks,
		"help": CmdHelp, "accuse": CmdAccuse, "quit": CmdQuit,
	}
	for line, want := range cases {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if cmd.Kind != want {
			t.Errorf("%q -> %v, ожидалось %v", line, cmd.Kind, want)
		}
	}
}

func TestParseTheorizeKeepsFreeText(t *testing.T) {
	cmd, err := Parse("theorize токе подменил запись в гроссбухе")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Intent.Args.Text != "токе подменил запись в гроссбухе" {
		t.Errorf("текст гипотезы искажён: %q", cmd.Intent.Args.Text)
	}
}

func TestParseRejectsUnknownVerb(t *testing.T) {
	if _, err := Parse("interrogate ivar"); err == nil {
		t.Error("неизвестный глагол принят")
	}
}

func TestParseIgnoresBlankAndComments(t *testing.T) {
	// Скриптовый режим читает файлы с комментариями — они не должны быть ходами.
	for _, line := range []string{"", "   ", "# это комментарий"} {
		cmd, err := Parse(line)
		if err != nil {
			t.Fatalf("%q дал ошибку %v", line, err)
		}
		if cmd.Kind != CmdNone {
			t.Errorf("%q разобрано как команда %v", line, cmd.Kind)
		}
	}
}

func TestParseRejectsWrongArity(t *testing.T) {
	if _, err := Parse("question ivar"); err == nil {
		t.Error("question без темы принят")
	}
	if _, err := Parse("compare alibi"); err == nil {
		t.Error("compare с одним фактом принят")
	}
}
```

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./cli/ -v`
Expected: FAIL — `undefined: Parse`

- [ ] **Step 3: Написать `cli/parse.go`**

```go
// Package cli — структурированный ввод и рендер. Никакого естественного языка:
// «question ivar ledger», а не «спрошу кузнеца про книгу».
package cli

import (
	"errors"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

type CommandKind int

const (
	CmdNone CommandKind = iota
	CmdAction
	CmdCompare
	CmdAccuse
	CmdFacts
	CmdState
	CmdClocks
	CmdHelp
	CmdQuit
)

type Command struct {
	Kind   CommandKind
	Intent core.Intent
	Facts  []store.FactID
	Text   string
}

var ErrUnknownVerb = errors.New("неизвестное действие")

// Parse разбирает одну строку ввода. Пустые строки и строки-комментарии дают
// CmdNone: скриптовый прогон читает те же файлы, что пишет человек.
func Parse(line string) (Command, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Command{Kind: CmdNone}, nil
	}
	fields := strings.Fields(line)
	head, rest := fields[0], fields[1:]

	switch head {
	case "facts":
		return Command{Kind: CmdFacts}, nil
	case "state":
		return Command{Kind: CmdState}, nil
	case "clocks":
		return Command{Kind: CmdClocks}, nil
	case "help":
		return Command{Kind: CmdHelp}, nil
	case "accuse":
		return Command{Kind: CmdAccuse}, nil
	case "quit", "exit":
		return Command{Kind: CmdQuit}, nil
	case "compare":
		if len(rest) != 2 {
			return Command{}, errors.New("compare требует ровно два факта")
		}
		return Command{Kind: CmdCompare, Facts: []store.FactID{
			factID(rest[0]), factID(rest[1]),
		}}, nil
	}

	def, ok := core.LookupVerb(head)
	if !ok {
		return Command{}, ErrUnknownVerb
	}

	cmd := Command{Kind: CmdAction, Intent: core.Intent{Verb: def.Verb}}
	switch head {
	case "theorize", "say", "emote":
		cmd.Intent.Args.Text = strings.Join(rest, " ")
		return cmd, nil
	case "move_zone":
		if len(rest) != 1 {
			return Command{}, errors.New("move_zone требует узел")
		}
		cmd.Intent.Args.Node = nodeID(rest[0])
		return cmd, nil
	case "question", "ask_about", "cross_reference":
		if len(rest) != 2 {
			return Command{}, errors.New(head + " требует источник и тему")
		}
		cmd.Intent.Args.Target = entityID(rest[0])
		cmd.Intent.Args.Topic = factID(rest[1])
		return cmd, nil
	case "use_item":
		if len(rest) != 1 {
			return Command{}, errors.New("use_item требует предмет")
		}
		cmd.Intent.Args.Item = rest[0]
		return cmd, nil
	case "use_ability":
		if len(rest) != 1 {
			return Command{}, errors.New("use_ability требует способность")
		}
		cmd.Intent.Args.Ability = rest[0]
		return cmd, nil
	case "look":
		return cmd, nil
	}

	if len(rest) != 1 {
		return Command{}, errors.New(head + " требует одну цель")
	}
	cmd.Intent.Args.Target = entityID(rest[0])
	return cmd, nil
}

// Префиксы можно не писать: «ivar» и «e_ivar» — одно и то же. Идентификаторы
// из вывода facts вставляются как есть, короткие имена набираются быстрее.
func entityID(s string) store.EntityID { return store.EntityID(withPrefix(s, "e_")) }
func factID(s string) store.FactID     { return store.FactID(withPrefix(s, "f_")) }
func nodeID(s string) store.NodeID     { return store.NodeID(withPrefix(s, "n_")) }

func withPrefix(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}
```

- [ ] **Step 4: Прогнать тесты, убедиться что проходят**

Run: `go test ./cli/ -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add cli/parse.go cli/parse_test.go
git commit -m "feat(cli): структурированный парсер команд с короткими идентификаторами"
```

---

### Task 20: Рендер, REPL, скриптовый режим и точка входа

**Files:**
- Create: `cli/render.go`, `cli/repl.go`, `cmd/dnd/main.go`
- Test: `cli/render_test.go`, `cli/repl_test.go`

**Interfaces:**
- Consumes: Tasks 12–19
- Produces: `cli.Render{}` с методами `Scene(*core.Game) string`, `Turn(*core.Game, core.TurnResult) string`, `Facts(*core.Game) string`, `State(*core.Game) string`, `Clocks(*core.Game) string`, `Help() string`; `cli.Session{Game *core.Game, In io.Reader, Out io.Writer}`, `cli.NewSession(*core.Game, io.Reader, io.Writer) *Session`, `(*Session).Run() error`

- [ ] **Step 1: Написать падающий тест рендера**

`cli/render_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

func TestRefusalReadsDifferentlyFromFailure(t *testing.T) {
	// Игрок обязан мгновенно видеть разницу: отказ не потратил ход.
	g := renderGame(t)
	r := Render{}
	refusal := r.Turn(g, refusedResult("парти об этом ничего не знает"))
	if !strings.Contains(refusal, "нельзя") {
		t.Errorf("отказ не помечен как отказ: %q", refusal)
	}
	if strings.Contains(refusal, "ПРОВАЛ") {
		t.Errorf("отказ подан как провал: %q", refusal)
	}
}

func TestFactsShowSourcesAndConfidence(t *testing.T) {
	g := renderGame(t)
	g.K.Learn("f_ligature", "e_body")
	g.K.Learn("f_ligature", "e_toke")
	out := Render{}.Facts(g)
	if !strings.Contains(out, "0.75") {
		t.Errorf("confidence не показан: %q", out)
	}
	if !strings.Contains(out, "e_body") || !strings.Contains(out, "e_toke") {
		t.Errorf("источники не показаны: %q", out)
	}
}

func TestStateShowsAttemptsAndGrit(t *testing.T) {
	g := renderGame(t)
	out := Render{}.State(g)
	for _, want := range []string{"узел", "grit", "попыт"} {
		if !strings.Contains(strings.ToLower(out), want) {
			t.Errorf("в state нет %q: %q", want, out)
		}
	}
}

func TestRenderNeverPrintsTruth(t *testing.T) {
	g := renderGame(t)
	all := Render{}.Scene(g) + Render{}.Facts(g) + Render{}.State(g) + Render{}.Clocks(g)
	for _, leak := range []string{"toke_is_killer", "<redacted>"} {
		if strings.Contains(all, leak) {
			t.Errorf("вывод содержит %q", leak)
		}
	}
}

func TestHelpListsEveryPlayerCommand(t *testing.T) {
	out := Render{}.Help()
	for _, cmd := range []string{"question", "examine", "search", "compare",
		"cross_reference", "stake_out", "theorize", "accuse", "facts", "state", "clocks"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("help не упоминает %q", cmd)
		}
	}
}
```

Вспомогательные функции `renderGame` и `refusedResult` — в том же файле: `renderGame` строит игру из `../cases/testdata/minimal.json` через `cases.Load` и `core.NewGame`, подставляя `threshold.New()` и `dice.NewSource(1).Stream("resolve")`; `refusedResult(msg)` возвращает `core.TurnResult{Refused: true, Refusal: msg}`.

- [ ] **Step 2: Написать падающий тест REPL**

`cli/repl_test.go`:

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestScriptModeRunsEveryLine(t *testing.T) {
	g := renderGame(t)
	in := strings.NewReader("look\nfacts\nstate\nquit\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("прогон не дал вывода")
	}
}

func TestUnknownCommandDoesNotStopTheRun(t *testing.T) {
	g := renderGame(t)
	in := strings.NewReader("interrogate ivar\nfacts\nquit\n")
	var out bytes.Buffer
	if err := NewSession(g, in, &out).Run(); err != nil {
		t.Fatalf("неизвестная команда уронила прогон: %v", err)
	}
	if !strings.Contains(out.String(), "нельзя") {
		t.Errorf("нет сообщения об отказе: %q", out.String())
	}
}

func TestSameSeedSameTranscript(t *testing.T) {
	// Пара (seed, скрипт) полностью задаёт вывод — это и есть харнесс.
	script := "look\nexamine body\nfacts\nquit\n"
	first := transcript(t, 7, script)
	second := transcript(t, 7, script)
	if first != second {
		t.Error("один seed дал разные транскрипты")
	}
	if other := transcript(t, 8, script); other == first {
		t.Log("разные seed дали одинаковый транскрипт — допустимо на коротком скрипте")
	}
}
```

Вспомогательная `transcript(t, seed, script)` строит игру с `dice.NewSource(seed)`, прогоняет `NewSession(...).Run()` и возвращает содержимое буфера.

- [ ] **Step 3: Прогнать тесты, убедиться что падают**

Run: `go test ./cli/ -run 'TestRefusal|TestScriptMode' -v`
Expected: FAIL — `undefined: Render`

- [ ] **Step 4: Написать `cli/render.go`**

```go
package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
)

type Render struct{}

func (Render) Scene(g *core.Game) string {
	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", g.DB.Locations[g.Node].Name)
	b.WriteString(g.Flavour("look."+string(g.Node)) + "\n")
	for _, e := range g.DB.EntitiesAt(g.Node) {
		fmt.Fprintf(&b, "  · %s (%s)\n", e.Name, e.ID)
	}
	if reach := g.ReachableNodes(); len(reach) > 0 {
		parts := make([]string, len(reach))
		for i, n := range reach {
			parts[i] = string(n)
		}
		fmt.Fprintf(&b, "  → %s\n", strings.Join(parts, ", "))
	}
	return b.String()
}

// Turn печатает исход хода. Отказ и провал оформлены по-разному намеренно:
// отказ не потратил ход, и игрок должен видеть это без раздумий.
func (r Render) Turn(g *core.Game, t core.TurnResult) string {
	if t.Refused {
		return "нельзя: " + t.Refusal + "\n"
	}
	var b strings.Builder
	if t.FlavourKey != "" {
		b.WriteString(g.Flavour(t.FlavourKey) + "\n")
	}
	if t.Res != nil {
		b.WriteString(r.roll(*t.Res))
	}
	for _, l := range t.Learned {
		fmt.Fprintf(&b, "  + узнали: %s (от %s)\n", g.DB.Facts[l.Fact].Key, l.From)
	}
	if t.FalseLead {
		b.WriteString("  ! след оказался ложным\n")
	}
	if t.HalfEffect {
		b.WriteString("  ~ помогло только наполовину\n")
	}
	for _, c := range t.Fired {
		b.WriteString("  ⏱ " + g.Flavour(c.FlavourKey) + "\n")
	}
	return b.String()
}

func (Render) roll(res core.Resolution) string {
	var terms []string
	for _, t := range res.Log.Terms {
		if t.Value != 0 {
			terms = append(terms, fmt.Sprintf("%s %+d", t.Name, t.Value))
		}
	}
	return fmt.Sprintf("  [d20=%d %s против %d] %s, маржа %+d\n",
		res.Log.Die, strings.Join(terms, " "), res.Log.Threshold, res.Class, res.Margin)
}

// Facts — единственный интерфейс к корроборации. По нему видно, зачем нужен
// третий независимый источник.
func (Render) Facts(g *core.Game) string {
	bank := g.K.TopicBank()
	if len(bank) == 0 {
		return "парти пока ничего не знает\n"
	}
	var b strings.Builder
	for _, f := range bank {
		srcs := g.K.Sources(f)
		names := make([]string, len(srcs))
		for i, s := range srcs {
			names[i] = string(s)
		}
		sort.Strings(names)
		mark := " "
		if g.K.Corroborated(f) {
			mark = "*"
		}
		fmt.Fprintf(&b, "%s %-24s %.3f  ← %s\n",
			mark, f, g.K.Confidence(f), strings.Join(names, ", "))
	}
	b.WriteString("(* — подтверждён тремя независимыми источниками)\n")
	return b.String()
}

func (Render) State(g *core.Game) string {
	ch := g.DB.Characters[g.Actor]
	return fmt.Sprintf("узел: %s\nранения: %d/3\ngrit: %d\nпопыток обвинения: %d\n",
		g.Node, ch.Harm, ch.Grit, g.Attempts)
}

func (Render) Clocks(g *core.Game) string {
	var b strings.Builder
	for _, c := range g.C.Snapshot() {
		fmt.Fprintf(&b, "%-20s [%s%s] %d/%d\n", c.Name,
			strings.Repeat("#", c.Filled), strings.Repeat(".", c.Segments-c.Filled),
			c.Filled, c.Segments)
	}
	return b.String()
}

func (Render) Help() string {
	return `question <источник> <тема>      расспросить
examine <предмет>               осмотреть
search <локация>                обыскать
compare <факт> <факт>           сопоставить
cross_reference <факт> <запись> сверить
stake_out <локация>             наблюдать
theorize <текст>                зафиксировать гипотезу
move_zone <узел>                перейти
accuse                          открыть форму обвинения
facts                           известные факты с источниками и confidence
state                           узел, часы, ранения, grit, попытки
clocks                          часы давления
help                            этот список
quit                            выйти
`
}
```

- [ ] **Step 5: Написать `cli/repl.go`**

```go
package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// Session гоняет один и тот же цикл и для интерактивного REPL, и для скрипта:
// прогон воспроизводится парой (seed, файл команд).
type Session struct {
	Game *core.Game
	In   io.Reader
	Out  io.Writer
	r    Render
}

func NewSession(g *core.Game, in io.Reader, out io.Writer) *Session {
	return &Session{Game: g, In: in, Out: out}
}

func (s *Session) Run() error {
	fmt.Fprint(s.Out, s.r.Scene(s.Game))
	sc := bufio.NewScanner(s.In)
	for sc.Scan() {
		cmd, err := Parse(sc.Text())
		if err != nil {
			fmt.Fprintf(s.Out, "нельзя: %v\n", err)
			continue
		}
		if done := s.dispatch(cmd); done {
			return nil
		}
	}
	return sc.Err()
}

func (s *Session) dispatch(cmd Command) bool {
	g, r := s.Game, s.r
	switch cmd.Kind {
	case CmdNone:
	case CmdQuit:
		return true
	case CmdHelp:
		fmt.Fprint(s.Out, r.Help())
	case CmdFacts:
		fmt.Fprint(s.Out, r.Facts(g))
	case CmdState:
		fmt.Fprint(s.Out, r.State(g))
	case CmdClocks:
		fmt.Fprint(s.Out, r.Clocks(g))
	case CmdCompare:
		fmt.Fprint(s.Out, r.Turn(g, g.Compare(cmd.Facts[0], cmd.Facts[1])))
	case CmdAccuse:
		s.accuse()
	case CmdAction:
		cmd.Intent.Actor = g.Actor
		res := g.Apply(cmd.Intent)
		fmt.Fprint(s.Out, r.Turn(g, res))
		if cmd.Intent.Verb == "move_zone" && res.Res != nil && res.Res.Class >= core.OutcomeSuccess {
			g.Node = cmd.Intent.Args.Node
			fmt.Fprint(s.Out, r.Scene(g))
		}
	}
	return false
}

// accuse собирает форму из доступных токенов. Ошибочная форма не сообщает,
// какой слот неверен: иначе слоты брутфорсятся по одному.
func (s *Session) accuse() {
	g := s.Game
	form := accusation.Form{}
	slots := []struct {
		name string
		dst  *store.Token
	}{
		{"who", &form.Who}, {"how", &form.How},
		{"when", &form.When}, {"why", &form.Why},
	}
	sc := bufio.NewScanner(s.In)
	for _, slot := range slots {
		avail := g.AvailableTokens(slot.name)
		if len(avail) == 0 {
			fmt.Fprintf(s.Out, "слот %s пуст: нужных фактов ещё нет\n", slot.name)
			return
		}
		parts := make([]string, len(avail))
		for i, a := range avail {
			parts[i] = string(a)
		}
		fmt.Fprintf(s.Out, "%s: %s\n> ", slot.name, strings.Join(parts, " | "))
		if !sc.Scan() {
			return
		}
		*slot.dst = store.Token(strings.TrimSpace(sc.Text()))
	}
	res := g.Accuse(form)
	switch {
	case res.Refused:
		fmt.Fprintf(s.Out, "нельзя: %s\n", res.Refusal)
	case res.Correct:
		fmt.Fprintf(s.Out, "Обвинение верно. Попыток: %d.\n", res.Attempt)
	default:
		fmt.Fprintf(s.Out, "В обвинении есть ошибки. Попыток: %d.\n", res.Attempt)
	}
	for _, c := range res.Fired {
		fmt.Fprintf(s.Out, "  ⏱ %s\n", g.Flavour(c.FlavourKey))
	}
}
```

- [ ] **Step 6: Написать `cmd/dnd/main.go`**

```go
// Команда dnd — консольный прототип детективной игры. Ни одного вызова LLM.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func main() {
	casePath := flag.String("case", "cases/harbour/case.json", "путь к файлу дела")
	seed := flag.Int64("seed", 1, "seed RNG: прогон воспроизводится парой (seed, ввод)")
	script := flag.String("script", "", "файл команд вместо интерактивного ввода")
	flag.Parse()

	cfg, err := cases.Load(*casePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(*seed).Stream("resolve")

	in := os.Stdin
	if *script != "" {
		f, err := os.Open(*script)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}

	if err := cli.NewSession(core.NewGame(*cfg), in, os.Stdout).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 7: Прогнать тесты и сборку**

Run: `go build ./... && go test ./... -v`
Expected: PASS

- [ ] **Step 8: Коммит**

```bash
git add cli/ cmd/
git commit -m "feat(cli): рендер сцены, REPL, скриптовый режим и точка входа"
```

---

### Task 21: Рукописное дело «Гавань»

**Files:**
- Create: `cases/harbour/case.json`
- Test: `cases/harbour/case_test.go`

**Interfaces:**
- Consumes: Tasks 17–18
- Produces: играбельное дело, проходящее `cases.Load` и валидатор

**Дело.** Сборщик податей Халден найден мёртвым на складе у пристани. Убийца — писарь гильдии Токе: Халден потребовал ревизию и нашёл подчистку в гроссбухе, которую делал Токе. Ложный подозреваемый — кузнец Ивар, который был Халдену должен и в ту ночь исчез из таверны.

**Правильный ответ (`truth`):** `who: toke`, `how: seal_cord`, `when: night_before_tide`, `why: audit_shortfall`.

**Локации:** `n_quay` (Пристань, теги `rain`), `n_warehouse` (Склад, `dark, indoors`), `n_forge` (Кузница, `indoors`), `n_guildhall` (Контора гильдии, `indoors`), `n_tavern` (Таверна, `crowd`). Смежность: пристань — со складом, кузницей и конторой; таверна — с кузницей.

**Сущности:** `e_body` (thing), `e_ledger` (record, в конторе), `e_lock` (thing, на складе), `e_toke` (npc, контора), `e_ivar` (npc, кузница), `e_sigrid` (npc, вдова Халдена, кузница), `e_nils` (npc, мальчишка-посыльный, пристань), `e_bern` (npc, стражник, пристань), `e_tidebook` (record, книга приливов, контора).

**Связи (`relations`)** — определяют, кто за независимые источники не считается: `e_sigrid ↔ e_nils` (тётка и племянник), `e_toke ↔ e_ledger` (гроссбух ведёт Токе), `e_ivar ↔ e_sigrid` (соседи по кузнице).

**Факты:**

| id | что | mandatory-держатель | gate |
|---|---|---|---|
| `f_body_found` | тело на складе | стартовый факт | — |
| `f_ligature` | борозда от шнура на шее | `e_body` | `examine`, easy |
| `f_seal_cord` | шнур — от печати гильдии | `e_body` | `examine`, normal, requires `f_ligature` |
| `f_lock_intact` | замок не взломан — был ключ | `e_lock` | `examine`, easy |
| `f_toke_has_key` | ключ есть у писаря | `e_bern` | `question`, normal |
| `f_ledger_erasure` | подчистка в гроссбухе | `e_ledger` | `examine`, normal |
| `f_shortfall` | недостача в кассе | `e_ledger` | `cross_reference`, hard, requires `f_ledger_erasure` |
| `f_toke_keeps_ledger` | гроссбух ведёт Токе | `e_toke` | `question`, easy |
| `f_halden_asked_audit` | Халден требовал ревизию | `e_sigrid` | `question`, normal |
| `f_tide_night` | склад заперт до прилива | `e_tidebook` | `examine`, easy |
| `f_toke_at_quay` | Токе видели у пристани ночью | `e_nils` | `stake_out`/`question`, normal, **latent** |
| `f_ivar_debt` | Ивар был должен Халдену | `e_sigrid` | `question`, easy |
| `f_ivar_alibi` | Ивар всю ночь был в таверне | `e_ivar` | `question`, normal |
| `f_toke_paid_debts` | Токе внезапно расплатился | `e_nils` | `question`, normal |
| `f_toke_lied` | вывод: Токе солгал о ночи | — | открывается `compare` |

**Разблокировки (`fact_unlocks`):** `f_ledger_erasure` открывает тему `f_toke_at_quay` (спящую) и узел `n_warehouse`; `f_seal_cord` открывает тему `f_toke_keeps_ledger`.

**Противоречие:** `f_ivar_alibi` × `f_ivar_debt` не противоречат — это ловушка ложного следа. Настоящее противоречие: `f_toke_at_quay` × `f_toke_keeps_ledger` → раскрывает `f_toke_lied`, ключ `compare.toke_lied`.

**Три независимых источника на каждый truth-факт** — обязательное условие валидатора. Для `f_shortfall`: `e_ledger`, `e_sigrid`, `e_bern` (между ними нет рёбер). Для `f_toke_at_quay`: `e_nils`, `e_bern`, `e_ivar`. Для `f_seal_cord`: `e_body`, `e_toke`, `e_bern`. Для `f_tide_night`: `e_tidebook`, `e_bern`, `e_ivar`.

**Часы:** `c_tide` («Прилив», 6 сегментов, `on_cost`) — при заполнении склад заливает, держатель `e_lock` исчезает (`remove_holders: ["f_lock_intact"]`), но `f_lock_intact` к этому моменту достижим и от `e_bern`. `c_suspicion` («Слухи», 8 сегментов, `on_cost`) — при заполнении Токе становится враждебен.

**Токены обвинения:** `who`: `toke` (под `f_toke_lied`), `ivar` (под `f_ivar_debt`), `sigrid` (под `f_halden_asked_audit`). `how`: `seal_cord` (под `f_seal_cord`), `hammer` (под `f_ivar_debt`). `when`: `night_before_tide` (под `f_tide_night`), `morning` (под `f_body_found`). `why`: `audit_shortfall` (под `f_shortfall`), `unpaid_debt` (под `f_ivar_debt`).

- [ ] **Step 1: Написать падающий тест**

`cases/harbour/case_test.go`:

```go
package harbour_test

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
)

func TestHarbourCaseLoadsAndValidates(t *testing.T) {
	if _, err := cases.Load("case.json"); err != nil {
		t.Fatalf("дело не проходит валидацию:\n%v", err)
	}
}

func TestFlavourIsRealProseNotStubs(t *testing.T) {
	// Заглушки дадут ложный негатив на главном вопросе M1a: с «TODO» вместо
	// текста расследование не может ощущаться игрой ни при какой механике.
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Flavour) < 30 {
		t.Errorf("текстов всего %d — дело недописано", len(cfg.Flavour))
	}
	for key, text := range cfg.Flavour {
		if len([]rune(text)) < 20 {
			t.Errorf("текст %q слишком короток: %q", key, text)
		}
		for _, stub := range []string{"TODO", "TBD", "заглушка", "lorem"} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(stub)) {
				t.Errorf("текст %q — заглушка: %q", key, text)
			}
		}
	}
}

func TestEveryTruthFactHasThreeIndependentSources(t *testing.T) {
	cfg, err := cases.Load("case.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"f_seal_cord", "f_shortfall", "f_toke_at_quay", "f_tide_night"} {
		var holders []string
		for _, h := range cfg.DB.Holders[storeFact(f)] {
			holders = append(holders, string(h.HolderID))
		}
		if len(holders) < 3 {
			t.Errorf("у %s всего %d источников: %v", f, len(holders), holders)
		}
	}
}
```

Вспомогательная `storeFact(s string) store.FactID` — тривиальная конверсия, объявить в том же файле.

- [ ] **Step 2: Прогнать тест, убедиться что падает**

Run: `go test ./cases/harbour/ -v`
Expected: FAIL — файла `case.json` нет

- [ ] **Step 3: Написать `cases/harbour/case.json`**

Собрать файл по таблицам выше в формате `cases.File` (Task 17). Требования к содержимому:

- **Флейвор пишется настоящей прозой.** Ключи: `look.<node>` для каждой из пяти локаций; `<verb>.<entity>.<fact>` для каждой пары держатель-факт; `<verb>.<entity>` для осмотров без факта; `compare.toke_lied`; `compare.nothing`; `clock.tide.filled`; `clock.suspicion.filled`. Каждый текст — две-четыре живых фразы, а не подпись к строке таблицы.
- **Голоса различаются.** У `e_bern` — служебная сухость, у `e_nils` — торопливость и лишние детали, у `e_sigrid` — усталая точность, у `e_toke` — вежливые уклонения, у `e_ivar` — раздражение. Поле `voice` заполняется и используется автором при письме.
- **Ложный след держится на правде.** Ивар действительно был должен и действительно исчез из таверны — просто не за тем. Ни одна реплика не лжёт игроку прямо, кроме реплик Токе о ночи.
- `start_facts`: `f_body_found` от `e_body`.
- `start`: `n_quay`. `character`: `mind 3`, `edge 1`, `body 0`, `will 0`, теги `портовый` (`question`, `search`) и `дознаватель` (`cross_reference`, `examine`), `grit 3`, архетип `inspector`.

- [ ] **Step 4: Прогнать тесты, убедиться что проходят**

Run: `go test ./cases/harbour/ -v`
Expected: PASS

- [ ] **Step 5: Сыграть дело руками и починить, что не играется**

Run: `go run ./cmd/dnd --case cases/harbour/case.json --seed 3`

Пройти дело от первой сцены до обвинения вручную. Это единственный шаг плана, где судит человек, а не тест: если расследование не ощущается игрой, дальше чинится дело, а не код.

- [ ] **Step 6: Коммит**

```bash
git add cases/harbour/
git commit -m "feat(cases): рукописное дело «Гавань» с настоящим флейвором"
```

---

### Task 22: Сквозные тесты — прохождение, достижимость, шов, отсутствие LLM

**Files:**
- Create: `cases/harbour/walkthrough.txt`, `e2e/walkthrough_test.go`, `e2e/reachability_test.go`, `e2e/architecture_test.go`
- Test: те же файлы

**Interfaces:**
- Consumes: всё предыдущее
- Produces: зелёный DoD

- [ ] **Step 1: Написать `cases/harbour/walkthrough.txt`**

Скрипт команд, проходящий дело от первой сцены до верного обвинения. Пишется после Task 21 руками, сверяясь с реальным выводом игры. Комментарии строками с `#` разрешены — парсер их пропускает.

- [ ] **Step 2: Написать тест прохождения**

`e2e/walkthrough_test.go`:

```go
package e2e

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func newGame(t *testing.T, seed int64) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	return core.NewGame(*cfg)
}

func TestWalkthroughReachesCorrectAccusation(t *testing.T) {
	script, err := os.ReadFile("../cases/harbour/walkthrough.txt")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	g := newGame(t, 3)
	if err := cli.NewSession(g, bytes.NewReader(script), &out).Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if !strings.Contains(out.String(), "Обвинение верно") {
		t.Fatalf("прохождение не дошло до верного обвинения:\n%s", out.String())
	}
}

func TestWalkthroughIsReproducible(t *testing.T) {
	script, _ := os.ReadFile("../cases/harbour/walkthrough.txt")
	run := func() string {
		var out bytes.Buffer
		cli.NewSession(newGame(t, 3), bytes.NewReader(script), &out).Run()
		return out.String()
	}
	if run() != run() {
		t.Error("один seed и один скрипт дали разные транскрипты")
	}
}
```

- [ ] **Step 3: Написать тест достижимости**

`e2e/reachability_test.go`. Двести прогонов случайного агента: инвариант — дело не может стать нерешаемым.

```go
package e2e

import (
	"math/rand"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// TestReachabilityNeverCollapses гоняет случайного агента и проверяет, что все
// четыре слота обвинения остаются собираемыми. Рукописное дело разрешимо по
// построению — этот тест ловит момент, когда правка данных это сломала.
func TestReachabilityNeverCollapses(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		g := newGame(t, seed)
		r := rand.New(rand.NewSource(seed))
		playRandomly(g, r, 120)

		if !slotsReachable(g) {
			t.Fatalf("seed %d: дело стало нерешаемым", seed)
		}
	}
}

// playRandomly делает случайные осмысленные ходы: перебирает известные темы и
// присутствующих держателей, иногда переходит между узлами.
func playRandomly(g *core.Game, r *rand.Rand, turns int) {
	verbs := []core.Verb{"examine", "question", "search", "stake_out", "cross_reference"}
	for i := 0; i < turns; i++ {
		here := g.DB.EntitiesAt(g.Node)
		bank := g.K.TopicBank()
		if len(here) == 0 || len(bank) == 0 {
			moveRandomly(g, r)
			continue
		}
		in := core.Intent{
			Verb:  verbs[r.Intn(len(verbs))],
			Actor: g.Actor,
			Args: core.Args{
				Target: here[r.Intn(len(here))].ID,
				Topic:  bank[r.Intn(len(bank))],
			},
		}
		g.Apply(in)
		tryCompares(g)
		if r.Intn(4) == 0 {
			moveRandomly(g, r)
		}
	}
}

func moveRandomly(g *core.Game, r *rand.Rand) {
	reach := g.ReachableNodes()
	if len(reach) == 0 {
		return
	}
	g.Node = reach[r.Intn(len(reach))]
}

func tryCompares(g *core.Game) {
	bank := g.K.TopicBank()
	for i := range bank {
		for j := i + 1; j < len(bank); j++ {
			g.Compare(bank[i], bank[j])
		}
	}
}

// slotsReachable проверяет, что каждый слот обвинения либо уже собираем, либо
// остаётся достижимым: держатель нужного факта жив и стоит в мире.
func slotsReachable(g *core.Game) bool {
	for _, f := range []store.FactID{
		"f_toke_lied", "f_seal_cord", "f_tide_night", "f_shortfall",
	} {
		if g.K.Knows(f) {
			continue
		}
		alive := false
		for _, h := range g.DB.HoldersOf(f) {
			if _, ok := g.DB.Entities[h.HolderID]; ok {
				alive = true
			}
		}
		if !alive && !revealedByCompare(g, f) {
			return false
		}
	}
	return true
}

func revealedByCompare(g *core.Game, f store.FactID) bool {
	for _, c := range g.DB.Contradictions {
		if c.Reveals == f {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Написать архитектурные тесты**

`e2e/architecture_test.go`. Шов и запрет на LLM проверяются механически — обещание в документе не проверяется ничем.

```go
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreDoesNotDependOnRules(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "../core").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, forbidden := range []string{"/rules/", "math/rand"} {
		if strings.Contains(string(out), forbidden) {
			t.Errorf("core тянет %q — шов протёк", forbidden)
		}
	}
}

func TestCoreMentionsNoDiceVocabulary(t *testing.T) {
	// Ядро не должно знать словарь системы правил даже на уровне строк.
	forbidden := []string{"d20", "grit +", "порог 14", "attribute"}
	err := filepath.Walk("../core", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, f := range forbidden {
			if strings.Contains(strings.ToLower(string(body)), f) {
				t.Errorf("%s содержит %q", path, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoLLMAndNoNetworkAnywhere(t *testing.T) {
	forbidden := []string{
		`"net/http"`, `"net"`, "anthropic", "openai", "completion(",
	}
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "docs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, f := range forbidden {
			if strings.Contains(strings.ToLower(string(body)), f) {
				t.Errorf("%s содержит %q — в M1a этого быть не может", path, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoExternalDependencies(t *testing.T) {
	body, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "require") {
		t.Errorf("появились внешние зависимости:\n%s", body)
	}
}
```

- [ ] **Step 5: Прогнать весь набор**

Run: `go test ./... -v`
Expected: PASS во всех пакетах

- [ ] **Step 6: Прогнать прохождение как игру**

Run: `go run ./cmd/dnd --case cases/harbour/case.json --seed 3 --script cases/harbour/walkthrough.txt`
Expected: вывод заканчивается строкой «Обвинение верно.»

- [ ] **Step 7: Коммит**

```bash
git add e2e/ cases/harbour/walkthrough.txt
git commit -m "test: сквозное прохождение, достижимость на 200 прогонах и проверки шва"
```

---

### Task 23: Отдых, гипотезы и покрытие всех глаголов

**Files:**
- Create: `core/rest.go`
- Modify: `core/game.go` — поле `Theories []string`; `core/turn.go` — обработка `theorize`; `cli/parse.go` — команда `rest`; `cli/repl.go` — диалог отдыха; `cli/render.go` — гипотезы в `state` и `rest` в `help`
- Test: `core/rest_test.go`, `core/allverbs_test.go`

**Interfaces:**
- Consumes: Tasks 8, 12, 13, 19, 20
- Produces: `core.RestKind` с `RestShort|RestLong`, `(*Game).RestPreview(RestKind) []store.ClockID`, `(*Game).Rest(RestKind) TurnResult`, `(*Game).Theories []string`, `cli.CmdRest`

**Почему отдельной задачей.** Спека §7.5 требует отдыха, а закрытый список CLI §10 команды под него не содержит: это дыра самой спеки, а не плана. Здесь она закрывается командой `rest short|long`. Длинный отдых — структурный переход: цена предъявляется **до** решения, иначе игрок платит вслепую.

- [ ] **Step 1: Написать падающий тест отдыха**

`core/rest_test.go`:

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func restGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 0, Harm: 2}
	db.Clocks["c_tide"] = &store.Clock{ID: "c_tide", Name: "Прилив", Segments: 6, TickPolicy: "on_cost"}
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth: accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func TestShortRestRestoresGritOnly(t *testing.T) {
	g := restGame()
	g.Rest(RestShort)
	ch := g.DB.Characters["pc"]
	if ch.Grit != GritMax {
		t.Errorf("grit = %d, ожидалось %d", ch.Grit, GritMax)
	}
	if ch.Harm != 2 {
		t.Errorf("короткий отдых снял ранение: harm = %d", ch.Harm)
	}
	if g.DB.Clocks["c_tide"].Filled != 0 {
		t.Error("короткий отдых тикнул часы")
	}
}

func TestLongRestHealsOneHarmAndTicks(t *testing.T) {
	g := restGame()
	g.Rest(RestLong)
	ch := g.DB.Characters["pc"]
	if ch.Harm != 1 {
		t.Errorf("harm = %d, ожидалось 1", ch.Harm)
	}
	if g.DB.Clocks["c_tide"].Filled != 1 {
		t.Errorf("длинный отдых не тикнул часы: %d", g.DB.Clocks["c_tide"].Filled)
	}
}

func TestRestPreviewNamesTheCostBeforePaying(t *testing.T) {
	g := restGame()
	got := g.RestPreview(RestLong)
	if len(got) != 1 || got[0] != "c_tide" {
		t.Errorf("предпросмотр цены = %v, ожидалось [c_tide]", got)
	}
	if g.DB.Clocks["c_tide"].Filled != 0 {
		t.Error("предпросмотр сам заплатил цену")
	}
	if len(g.RestPreview(RestShort)) != 0 {
		t.Error("короткий отдых объявил цену")
	}
}

func TestLongRestWithoutHarmStillCosts(t *testing.T) {
	// Длинный отдых — структурный переход, а не арифметика: время идёт даже
	// у здорового.
	g := restGame()
	g.DB.Characters["pc"].Harm = 0
	g.Rest(RestLong)
	if g.DB.Clocks["c_tide"].Filled != 1 {
		t.Error("длинный отдых без ранений обошёлся бесплатно")
	}
}
```

- [ ] **Step 2: Написать тест покрытия всех глаголов**

`core/allverbs_test.go`. Спека §11 требует, чтобы ни один глагол не упирался в необработанный путь.

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// TestEveryVerbResolvesWithoutPanic прогоняет все 30 глаголов через Apply на
// осмысленных аргументах. Проверяется не исход, а отсутствие дыр: паники,
// нулевого результата, необработанной ветки.
func TestEveryVerbResolvesWithoutPanic(t *testing.T) {
	for _, def := range AllVerbs() {
		t.Run(string(def.Verb), func(t *testing.T) {
			g := turnGame(OutcomeSuccess)
			g.K.Learn("f_open", "e_bern")
			in := Intent{Verb: def.Verb, Actor: g.Actor, Args: Args{
				Target: "e_toke", Topic: "f_open", Node: "n_forge",
				Item: "crowbar", Ability: "read_room", Text: "гипотеза",
			}}
			got := g.Apply(in)
			if got.Refused && got.Refusal == "" {
				t.Error("отказ без причины — необработанная ветка")
			}
			if !got.Refused && got.FlavourKey == "" {
				t.Error("действие прошло, но не дало ключа флейвора")
			}
		})
	}
}

func TestTheorizeIsRecordedAndCostsNothing(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	got := g.Apply(Intent{Verb: "theorize", Actor: g.Actor,
		Args: Args{Text: "Токе подменил запись"}})
	if got.Refused {
		t.Fatalf("гипотеза отвергнута: %s", got.Refusal)
	}
	if len(g.Theories) != 1 || g.Theories[0] != "Токе подменил запись" {
		t.Errorf("гипотеза не записана: %v", g.Theories)
	}
	if len(got.Costs) != 0 {
		t.Errorf("рассуждение стоило %v", got.Costs)
	}
	if g.DB.Clocks["c_suspicion"].Filled != 0 {
		t.Error("гипотеза тикнула часы")
	}
}

var _ = store.FactID("")
```

- [ ] **Step 3: Прогнать тесты, убедиться что падают**

Run: `go test ./core/ -run 'TestShortRest|TestLongRest|TestRestPreview|TestEveryVerbResolves|TestTheorize' -v`
Expected: FAIL — `undefined: RestShort`

- [ ] **Step 4: Написать `core/rest.go`**

```go
package core

import "github.com/kliuchnikovv/dnd/store"

type RestKind int

const (
	RestShort RestKind = iota
	RestLong
)

// GritMax — потолок push-ресурса. Значение принадлежит системе правил, но
// восстановление отдыхом — структурный переход ядра, поэтому потолок объявлен
// здесь и переопределяется данными дела в M1b.
const GritMax = 3

// RestPreview называет часы, которые продвинет длинный отдых. Цена
// предъявляется до решения: платить вслепую игрок не должен.
func (g *Game) RestPreview(kind RestKind) []store.ClockID {
	if kind != RestLong {
		return nil
	}
	var out []store.ClockID
	for _, c := range g.C.Snapshot() {
		if c.TickPolicy == "on_cost" && c.Filled < c.Segments {
			out = append(out, c.ID)
		}
	}
	return out
}

// Rest — короткий возвращает grit, длинный снимает ячейку ранений ценой тика.
func (g *Game) Rest(kind RestKind) TurnResult {
	ch := g.DB.Characters[g.Actor]
	if ch == nil {
		return refuse("некому отдыхать")
	}
	if kind == RestShort {
		ch.Grit = GritMax
		return TurnResult{FlavourKey: "rest.short"}
	}
	if ch.Harm > 0 {
		ch.Harm--
	}
	out := TurnResult{FlavourKey: "rest.long"}
	out.Fired = g.C.TickAll(1)
	g.applyConsequences(out.Fired)
	return out
}
```

- [ ] **Step 5: Записывать гипотезы**

В `core/game.go` добавить в структуру `Game`:

```go
	Theories []string
```

В `core/turn.go`, в начале `Apply`, сразу после разбора `def`:

```go
	if in.Verb == "theorize" {
		if in.Args.Text == "" {
			return refuse("гипотеза не может быть пустой")
		}
		g.Theories = append(g.Theories, in.Args.Text)
		return TurnResult{FlavourKey: "theorize.recorded"}
	}
```

- [ ] **Step 6: Добавить `rest` в CLI**

В `cli/parse.go` — новый вид команды и её разбор:

```go
	CmdRest
```

```go
	case "rest":
		if len(rest) != 1 || (rest[0] != "short" && rest[0] != "long") {
			return Command{}, errors.New("rest требует short или long")
		}
		return Command{Kind: CmdRest, Text: rest[0]}, nil
```

В `cli/repl.go`, в `dispatch`:

```go
	case CmdRest:
		kind := core.RestShort
		if cmd.Text == "long" {
			kind = core.RestLong
		}
		if clocks := g.Game.RestPreview(kind); len(clocks) > 0 {
			fmt.Fprintf(s.Out, "длинный отдых продвинет часы: %v\n", clocks)
		}
		fmt.Fprint(s.Out, r.Turn(g, g.Rest(kind)))
```

(в теле `dispatch` переменная игры называется `g` — использовать `g.RestPreview`)

В `cli/render.go` — строка в `Help()`:

```
rest short|long                 короткий или длинный отдых
```

и вывод гипотез в `State`:

```go
	if len(g.Theories) > 0 {
		b := &strings.Builder{}
		fmt.Fprintf(b, "гипотезы:\n")
		for i, th := range g.Theories {
			fmt.Fprintf(b, "  %d. %s\n", i+1, th)
		}
		out += b.String()
	}
```

(перевести `State` на `strings.Builder` целиком, если так чище)

- [ ] **Step 7: Добавить тексты отдыха в дело**

В `cases/harbour/case.json` — ключи `rest.short`, `rest.long`, `theorize.recorded` настоящей прозой. Валидатор их не требует, но `Flavour` вернёт `[ключ]` в выводе, и это заметно сразу.

- [ ] **Step 8: Прогнать весь набор**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 9: Коммит**

```bash
git add core/rest.go core/rest_test.go core/allverbs_test.go core/game.go core/turn.go cli/ cases/harbour/case.json
git commit -m "feat: отдых, запись гипотез и покрытие всех 30 глаголов"
```

---

## Definition of done

- [ ] `go test ./...` зелёный целиком
- [ ] `go run ./cmd/dnd --case cases/harbour/case.json --seed 3 --script cases/harbour/walkthrough.txt` заканчивается верным обвинением
- [ ] Дело проходится вручную через CLI от первой сцены до обвинения
- [ ] `go.mod` без единой внешней зависимости
- [ ] Ни одного вызова LLM и ни одного сетевого импорта
