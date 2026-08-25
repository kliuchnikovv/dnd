# Знание мест. План реализации

> **Исполнителю:** ОБЯЗАТЕЛЬНЫЙ ПОДНАВЫК — `superpowers:subagent-driven-development`
> (рекомендуется) либо `superpowers:executing-plans`. Шаги отмечаются чекбоксами.

**Цель:** перевести гейт перемещения с «разрешено войти» на «место известно» —
так, что рассказанная персонажем дорога открывает место через границу мутаций.

**Подход:** новая таблица `known_places` в сторе (отдельно от `party_knowledge`);
`ReachableNodes` начинает означать «известные места дела»; `nodeLocked`
исчезает; `fact_unlocks` вида `node` меняет смысл на «место стало известно»;
персонаж объявляет рассказанное поле́м ответа, оно проходит капабилити-гейт и
инвариант ядра.

**Стек:** Go 1.26, `go test ./...`, без новых зависимостей.

**Спека:** `docs/superpowers/specs/2026-08-25-known-places-design.md` — читать
вместе с планом, план спорит из неё.

## Общие ограничения

- Состояние меняется только через `core.Apply` или `core.ProposeMutation`.
  Третьей двери нет; стережёт `e2e/border_test.go`.
- `core` не импортирует `llm`. Капабилити живут в `propose`.
- Знание мест — таблица, отдельная от `party_knowledge`. Недоверенный путь не
  касается фактов дела, токенов и правды.
- Персонаж делает известным только **смежное текущему узлу** место.
- `go test ./...`, `go vet ./...`, `gofmt -l .` чисты после каждой задачи.
- Коммит на зелёном, по одному на задачу.
- Тесты и комментарии по-русски, как весь репозиторий.

**Важно про рабочее дерево:** в нём лежит незакоммиченная работа в `actor/`,
`cli/`, `tui/`, `core/turn.go`, `intent/`, `docs/status.md`. Перед задачей,
которая трогает такой файл, собирать коммит только из своих хунков (проверить
`git diff --stat <файл>` до и после).

---

### Задача 1: стор — таблица known_places

**Файлы:**
- Создать: `store/places.go`
- Изменить: `store/db.go:7-40` (поле), `store/db.go:42-58` (`NewDB`)
- Тест: `store/places_test.go`

**Интерфейсы:**
- Отдаёт: `store.PlaceKey{PartyID string, CaseID CaseID, NodeID NodeID}`;
  `(*DB).KnowsPlace(party string, c CaseID, n NodeID) bool`;
  `(*DB).KnowPlace(party string, c CaseID, n NodeID)`;
  `(*DB).PlacesOf(party string, c CaseID) []NodeID` — в порядке возрастания id.

- [ ] **Шаг 1: написать падающий тест**

```go
package store

import "testing"

// Знание места — таблица, а не флаг у локации: одно и то же место известно
// одной парти и неизвестно другой, и в общем мире это станет обычным делом.
func TestKnownPlaceIsPerPartyAndCase(t *testing.T) {
	db := NewDB()
	db.KnowPlace("party", "harbour", "n_warehouse")

	if !db.KnowsPlace("party", "harbour", "n_warehouse") {
		t.Error("записанное место не читается")
	}
	if db.KnowsPlace("other", "harbour", "n_warehouse") {
		t.Error("знание одной парти видно другой")
	}
	if db.KnowsPlace("party", "forte_merlo", "n_warehouse") {
		t.Error("знание одного дела видно в другом")
	}
}

// Повтор — не ошибка: реплей и второй рассказ обязаны сходиться.
func TestKnowPlaceIsIdempotent(t *testing.T) {
	db := NewDB()
	db.KnowPlace("party", "harbour", "n_quay")
	db.KnowPlace("party", "harbour", "n_quay")
	if got := db.PlacesOf("party", "harbour"); len(got) != 1 {
		t.Errorf("повтор удвоил запись: %v", got)
	}
}

// Порядок стабилен: список мест печатается игроку и уезжает в промпт, а
// итерация по map случайна.
func TestPlacesReadInStableOrder(t *testing.T) {
	db := NewDB()
	for _, n := range []NodeID{"n_quay", "n_forge", "n_warehouse"} {
		db.KnowPlace("party", "harbour", n)
	}
	got := db.PlacesOf("party", "harbour")
	want := []NodeID{"n_forge", "n_quay", "n_warehouse"}
	if len(got) != len(want) {
		t.Fatalf("мест %d, ожидалось %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("порядок нестабилен: %v", got)
		}
	}
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Запуск: `go test ./store/ -run "KnownPlace|KnowPlace|PlacesRead"`
Ожидание: не компилируется — `db.KnowPlace undefined`.

- [ ] **Шаг 3: реализовать**

`store/places.go`:

```go
package store

import "sort"

// PlaceKey — ключ таблицы known_places: чья парти, какое дело, какое место.
// Дело в ключе потому же, что и у канона: место, известное в одном деле, не
// становится известным в другом.
type PlaceKey struct {
	PartyID string
	CaseID  CaseID
	NodeID  NodeID
}

// Знание мест — ОТДЕЛЬНАЯ таблица от party_knowledge, и это конструктивно.
// Место — публичная география: оно ничего не доказывает, токенов не открывает
// и в речь обвинения не входит. Держать его среди фактов дела значило бы
// стереть эту разницу и открыть недоверенному слою дверь к уликам.
func (db *DB) KnowsPlace(party string, c CaseID, n NodeID) bool {
	return db.KnownPlaces[PlaceKey{PartyID: party, CaseID: c, NodeID: n}]
}

// KnowPlace помечает место известным. Повтор — no-op: реплей и второй рассказ
// обязаны сходиться.
func (db *DB) KnowPlace(party string, c CaseID, n NodeID) {
	db.KnownPlaces[PlaceKey{PartyID: party, CaseID: c, NodeID: n}] = true
}

// PlacesOf — известные места в стабильном порядке. Итерация по map случайна, а
// список печатается игроку и уезжает в промпт парсера.
func (db *DB) PlacesOf(party string, c CaseID) []NodeID {
	var out []NodeID
	for k := range db.KnownPlaces {
		if k.PartyID == party && k.CaseID == c {
			out = append(out, k.NodeID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
```

В `store/db.go` в структуру `DB` рядом с `Canon`:

```go
	// KnownPlaces — места, о которых парти знает. Отдельно от party_knowledge:
	// место не улика (см. store/places.go).
	KnownPlaces map[PlaceKey]bool
```

и в `NewDB()` рядом с `Canon: map[CanonKey]CanonFact{},`:

```go
		KnownPlaces: map[PlaceKey]bool{},
```

- [ ] **Шаг 4: тесты зелёные**

Запуск: `go test ./store/`
Ожидание: PASS.

- [ ] **Шаг 5: закрыть третью дверь**

`KnowPlace` — экспортированный мутирующий метод стора, то есть ровно та третья
дверь, которую стережёт `e2e/border_test.go`. Вписать его в `worldWrites`:

```go
	"KnowPlace":       "выдача знания о месте",
```

Запуск: `go test ./e2e/ -run TestStateIsWrittenOnlyThroughTheBorder`
Ожидание: PASS — снаружи домена его пока никто не звал.

- [ ] **Шаг 6: коммит**

```bash
git add store/places.go store/places_test.go store/db.go e2e/border_test.go
git commit -m "feat(store): таблица known_places — знание мест отдельно от улик"
```

---

### Задача 2: ядро — известность и её авторские источники

**Файлы:**
- Создать: `core/places.go`
- Изменить: `core/game.go:25-50` (`Config`), `core/game.go:118-135` (`NewGame`),
  `core/unlocks.go:11-19` (`applyUnlocksFor`)
- Тест: `core/places_test.go`

**Интерфейсы:**
- Потребляет: `store.DB.KnowsPlace/KnowPlace/PlacesOf` из задачи 1.
- Отдаёт: `(*Game).KnowsPlace(n store.NodeID) bool`;
  `(*Game).knowPlace(n store.NodeID) bool` — неэкспортированный писатель,
  возвращает «стало известно впервые»;
  `(*Game).KnownPlaces() []store.NodeID`;
  поле `Config.StartPlaces []store.NodeID`.

- [ ] **Шаг 1: написать падающий тест**

```go
package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func placesGame(start ...store.NodeID) *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay",
		Adjacent: []store.NodeID{"n_warehouse", "n_forge"}}
	db.Locations["n_warehouse"] = store.Location{ID: "n_warehouse",
		Adjacent: []store.NodeID{"n_quay"}}
	db.Locations["n_forge"] = store.Location{ID: "n_forge",
		Adjacent: []store.NodeID{"n_quay", "n_tavern"}}
	db.Locations["n_tavern"] = store.Location{ID: "n_tavern",
		Adjacent: []store.NodeID{"n_forge"}}
	db.Facts["f_ledger"] = store.Fact{ID: "f_ledger", Key: "подчистка в гроссбухе"}
	db.Unlocks["f_ledger"] = []store.FactUnlock{
		{FactID: "f_ledger", UnlocksKind: "node", UnlocksID: "n_tavern"},
	}
	return NewGame(Config{DB: db, CaseID: "harbour", Start: "n_quay",
		StartPlaces: start})
}

// Место, где игрок стоит, известно всегда: иначе он не знает, где он.
func TestStartNodeIsKnown(t *testing.T) {
	g := placesGame()
	if !g.KnowsPlace("n_quay") {
		t.Error("стартовый узел неизвестен")
	}
	if g.KnowsPlace("n_warehouse") {
		t.Error("смежное место известно само по себе — тогда знание ничего не гейтит")
	}
}

// Автор вправе объявить место известным с начала: брифинг «Гавани» называет
// склад первой строкой, и не знать о нём игрок не может.
func TestStartPlacesAreKnown(t *testing.T) {
	g := placesGame("n_warehouse")
	if !g.KnowsPlace("n_warehouse") {
		t.Error("объявленное автором место неизвестно")
	}
}

// fact_unlocks вида node в новом смысле: узнал факт — узнал о месте.
func TestLearnedFactMakesPlaceKnown(t *testing.T) {
	g := placesGame()
	if g.KnowsPlace("n_tavern") {
		t.Fatal("место известно до факта")
	}
	g.applyUnlocksFor("f_ledger")
	if !g.KnowsPlace("n_tavern") {
		t.Error("факт не открыл место")
	}
}

// Повтор возвращает «уже знали»: на этом стоит идемпотентность мутации.
func TestKnowPlaceReportsFirstTimeOnly(t *testing.T) {
	g := placesGame()
	if !g.knowPlace("n_forge") {
		t.Error("первый рассказ не считается новым")
	}
	if g.knowPlace("n_forge") {
		t.Error("повтор объявлен новым")
	}
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Запуск: `go test ./core/ -run "StartNodeIsKnown|StartPlaces|LearnedFactMakes|KnowPlaceReports"`
Ожидание: не компилируется — `unknown field StartPlaces`, `g.KnowsPlace undefined`.

- [ ] **Шаг 3: реализовать**

`core/places.go`:

```go
package core

import "github.com/kliuchnikovv/dnd/store"

// Знание мест. Гейт перемещения стоит здесь, и он один: место известно или
// неизвестно, «разрешено» как понятие не существует.
//
// Почему знание, а не разрешение: игрок, которому только что объяснили дорогу,
// не должен получать отказ. Пока гейт стоял на разрешении, рассказ персонажа
// не менял мира — импровизация оставалась украшением.

// KnowsPlace — знает ли парти это место.
func (g *Game) KnowsPlace(n store.NodeID) bool {
	return g.DB.KnowsPlace(g.party, g.CaseID, n)
}

// knowPlace помечает место известным и отвечает, случилось ли это впервые.
//
// Неэкспортирован намеренно: снаружи домена место делает известным только
// ProposeMutation, где проверяется смежность. Экспортированный писатель был бы
// второй дверью — без проверки.
func (g *Game) knowPlace(n store.NodeID) bool {
	if g.KnowsPlace(n) {
		return false
	}
	g.DB.KnowPlace(g.party, g.CaseID, n)
	return true
}

// KnownPlaces — известные места этого дела в стабильном порядке.
func (g *Game) KnownPlaces() []store.NodeID {
	return g.DB.PlacesOf(g.party, g.CaseID)
}
```

В `Config` (`core/game.go`) после `Start`:

```go
	// StartPlaces — места, о которых игрок знает с начала: их назвал брифинг.
	// Поле явное, а не выведенное из его текста: связь прозы с узлом
	// машиночитаемой не бывает, а догадка по подстроке — это второй парсер.
	StartPlaces []store.NodeID
```

В `NewGame` после создания `g` (потребуется завести переменную вместо
возврата литерала — узел и места помечаются уже на собранной игре):

```go
	g := &Game{ /* существующий литерал без изменений */ }
	// Где стоишь — то знаешь. Дальше добавляется только объявленное автором:
	// смежность сама по себе места не открывает, иначе рассказ персонажа
	// ничего бы не решал.
	g.knowPlace(cfg.Start)
	for _, n := range cfg.StartPlaces {
		g.knowPlace(n)
	}
	return g
```

В `applyUnlocksFor` (`core/unlocks.go`) — вид `node` больше не пишет в
`g.unlocked`:

```go
	for _, u := range g.DB.Unlocks[f] {
		if u.UnlocksKind == "node" {
			// Новый смысл: факт не «разрешает войти», а рассказывает о месте.
			g.knowPlace(store.NodeID(u.UnlocksID))
			continue
		}
		g.unlocked[u.UnlocksKind+":"+u.UnlocksID] = true
	}
```

- [ ] **Шаг 4: тесты зелёные**

Запуск: `go test ./core/ -run "Place|Unlock"`
Ожидание: новые PASS. `TestLearningUnlocksTopicAndNode` и
`TestReachableNodesRespectLocks` в `core/unlocks_test.go` упадут — это
ожидаемо, их переписывает задача 3.

- [ ] **Шаг 5: коммит**

```bash
git add core/places.go core/places_test.go core/game.go core/unlocks.go
git commit -m "feat(core): известность мест и её источники — старт, автор, факт"
```

---

### Задача 3: ядро — перемещение по знанию, `nodeLocked` уходит

**Файлы:**
- Изменить: `core/unlocks.go:23-45` (`ReachableNodes`, удалить `nodeLocked`),
  `core/turn.go:183-190` (ветка узла в `validate`)
- Тест: `core/unlocks_test.go:44-90` (переписать два теста), `core/places_test.go`

**Интерфейсы:**
- Потребляет: `(*Game).KnowsPlace`, `(*Game).KnownPlaces` из задачи 2.
- Отдаёт: `ReachableNodes()` в новом значении — известные места дела за
  вычетом текущего.

- [ ] **Шаг 1: написать падающий тест**

В `core/places_test.go` добавить:

```go
// Идти можно в любое известное место, смежное или нет: посёлок маленький, ноги
// есть. Граф остаётся для прозы дороги и для того, о чём вправе рассказать
// персонаж, — но стеной быть перестаёт.
func TestMoveGoesToAnyKnownPlace(t *testing.T) {
	g := placesGame()
	g.applyUnlocksFor("f_ledger") // таверна известна, но не смежна пристани

	got := g.Apply(Intent{Verb: "move_zone", Args: Args{Node: "n_tavern"}})
	if got.Refused {
		t.Errorf("несмежное известное место отвергнуто: %s", got.Refusal)
	}
}

// Неизвестное место недоступно незнанием, а не запретом. Отказ не подсказывает,
// чем открыть, — как и все отказы гейтов.
func TestMoveToUnknownPlaceIsRefused(t *testing.T) {
	g := placesGame()
	got := g.Apply(Intent{Verb: "move_zone", Args: Args{Node: "n_warehouse"}})
	if !got.Refused {
		t.Fatal("неизвестное место пропущено")
	}
	for _, leak := range []string{"f_ledger", "узна", "спрос"} {
		if strings.Contains(got.Refusal, leak) {
			t.Errorf("отказ подсказывает, чем открыть: %q", got.Refusal)
		}
	}
}

// Список доступного — известные места дела без того, где игрок стоит.
func TestReachableIsKnownPlacesWithoutCurrent(t *testing.T) {
	g := placesGame("n_warehouse")
	got := g.ReachableNodes()
	if len(got) != 1 || got[0] != "n_warehouse" {
		t.Errorf("список доступного = %v, ожидалось [n_warehouse]", got)
	}
}
```

Импорт `"strings"` в `core/places_test.go` добавить.

В `core/unlocks_test.go` заменить `TestLearningUnlocksTopicAndNode` и
`TestReachableNodesRespectLocks` на:

```go
// Факт открывает тему через unlocked и место через known_places: два разных
// хранилища, потому что тема — про дело, а место — про географию.
func TestLearningUnlocksTopicAndOpensPlace(t *testing.T) {
	g := unlocksGame() // существующий хелпер файла
	if g.KnowsPlace("n_cellar") {
		t.Fatal("место известно до факта")
	}
	g.applyUnlocksFor("f_deep")
	if !g.Unlocked("topic", "f_deep") {
		t.Error("тема не открыта")
	}
	if !g.KnowsPlace("n_cellar") {
		t.Error("место не стало известным")
	}
}
```

(Хелпер и имена фактов взять из текущего `core/unlocks_test.go:1-45`; если
`unlocksGame` там называется иначе — использовать существующее имя.)

- [ ] **Шаг 2: убедиться, что тест падает**

Запуск: `go test ./core/`
Ожидание: FAIL — `move_zone` в несмежное место отвергается, `ReachableNodes`
возвращает смежные.

- [ ] **Шаг 3: реализовать**

`core/unlocks.go` — заменить `ReachableNodes` и удалить `nodeLocked`:

```go
// ReachableNodes — места, куда можно пойти: известные парти, кроме того, где
// игрок стоит. Смежность здесь не при чём — гейт стоит на знании (core/places.go).
func (g *Game) ReachableNodes() []store.NodeID {
	var out []store.NodeID
	for _, n := range g.KnownPlaces() {
		if n != g.Node {
			out = append(out, n)
		}
	}
	return out
}
```

Импорт `sort` из `core/unlocks.go` убрать: порядок даёт `PlacesOf`.

`core/turn.go`, ветка узла в `validate` — вместо смежности и замка:

```go
	if in.Args.Node != "" && !g.KnowsPlace(in.Args.Node) {
		return refuse("ты не знаешь, где это"), true
	}
```

- [ ] **Шаг 4: тесты зелёные**

Запуск: `go test ./core/`
Ожидание: PASS. Затем `go test ./...`.

`cli/render.go` (выходы сцены) и `intent/hint.go` (`SceneHint.Reachable`,
перечисление узлов в схеме) читают `ReachableNodes()` и следуют за новым
значением сами — правок в них не нужно, и это и есть инвариант §8.5 спеки: один
список доступного на всю игру. Их тесты, если падают, падают на ожиданиях
(ждали смежные) — править тесты. Падения в `e2e` закрывает задача 9.
Записать список упавших тестов в сообщение коммита, чтобы следующая задача
знала, что чинит.

- [ ] **Шаг 5: коммит**

```bash
git add core/unlocks.go core/turn.go core/places_test.go core/unlocks_test.go
git commit -m "feat(core): перемещение по знанию места — nodeLocked уходит"
```

---

### Задача 4: `MutPlaceKnown` на недоверенном пути

**Файлы:**
- Изменить: `core/resolution.go:65-90` (вид), `core/mutate.go:118-160`
  (`ProposeMutation`, новый валидатор)
- Тест: `core/mutate_test.go`

**Интерфейсы:**
- Потребляет: `(*Game).knowPlace` из задачи 2.
- Отдаёт: `core.MutPlaceKnown MutationKind = "place_known"`; в `Applied` при
  успехе `Kind = MutPlaceKnown`, `Target` = узел.

- [ ] **Шаг 1: написать падающий тест**

```go
// Место, о котором рассказали, становится известным через границу — и только
// смежное текущему узлу. Иначе одна болтливая реплика раскрыла бы карту, и
// разведка перестала бы быть занятием.
func TestProposedPlaceMustBeAdjacent(t *testing.T) {
	g := placesGame() // из core/places_test.go, игрок на n_quay
	app, ref := g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_warehouse"})
	if ref.Refused() {
		t.Fatalf("смежное место отвергнуто: %q", ref.Reason)
	}
	if app.Kind != MutPlaceKnown || app.Target != "n_warehouse" {
		t.Errorf("применено не то: %+v", app)
	}
	if !g.KnowsPlace("n_warehouse") {
		t.Error("место не стало известным")
	}

	// Таверна смежна кузнице, а не пристани: отсюда о ней рассказать нельзя.
	if _, ref := g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_tavern"}); !ref.Refused() {
		t.Error("несмежное место прошло")
	}
	// Несуществующего места не бывает даже в рассказе.
	if _, ref := g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_нет"}); !ref.Refused() {
		t.Error("несуществующий узел прошёл")
	}
}

// Знание места не касается улик. Инвариант §4 спеки: место — публичная
// география, и попасть в таблицу фактов оно не вправе даже случайно.
func TestProposedPlaceTouchesNoKnowledge(t *testing.T) {
	g := placesGame()
	before := len(g.DB.Knowledge)
	g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_warehouse"})
	if len(g.DB.Knowledge) != before {
		t.Errorf("рассказ о месте написал в party_knowledge: %+v", g.DB.Knowledge)
	}
	if len(g.K.TopicBank()) != 0 {
		t.Errorf("место появилось в банке тем: %v", g.K.TopicBank())
	}
}

// Повтор — идемпотентность, а не ошибка: персонаж вправе рассказать дорогу
// второй раз, и это не отказ.
func TestProposedPlaceRepeatIsApplied(t *testing.T) {
	g := placesGame()
	g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_forge"})
	app, ref := g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_forge"})
	if ref.Refused() || app.Kind != MutPlaceKnown {
		t.Errorf("повтор отвергнут: %q / %+v", ref.Reason, app)
	}
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Запуск: `go test ./core/ -run "ProposedPlace"`
Ожидание: не компилируется — `undefined: MutPlaceKnown`.

- [ ] **Шаг 3: реализовать**

В блок констант `MutationKind` (`core/resolution.go`), к недоверенным видам:

```go
	// MutPlaceKnown — место, о котором персонаж рассказал игроку. Target это
	// узел, Text не используется. Пишет в known_places и никогда в факты дела:
	// место — публичная география, оно ничего не доказывает.
	MutPlaceKnown MutationKind = "place_known"
```

В `ProposeMutation` (`core/mutate.go`) новый case перед числовыми:

```go
	case MutPlaceKnown:
		return g.proposePlace(m)
```

и валидатор рядом с `proposeCanon`:

```go
// proposePlace делает место известным по рассказу персонажа.
//
// Смежность текущему узлу — единственное, что здесь стережёт карту: рассказать
// можно про то, куда отсюда ведёт дорога. Перепрыгнуть через полкарты одной
// репликой нельзя, и разведка остаётся занятием.
func (g *Game) proposePlace(m Mutation) (Applied, Refusal) {
	n := store.NodeID(m.Target)
	if _, ok := g.DB.Locations[n]; !ok {
		return refuseMutation(m, "такого места в деле нет")
	}
	if !g.DB.Adjacent(g.Node, n) {
		return refuseMutation(m, "отсюда дорога туда не ведёт")
	}
	// Повтор идемпотентен: рассказать дорогу второй раз не ошибка.
	g.knowPlace(n)
	return Applied{Kind: MutPlaceKnown, Target: string(n)}, Refusal{}
}
```

- [ ] **Шаг 4: тесты зелёные**

Запуск: `go test ./core/ -run "Propose"`
Ожидание: PASS, включая существующие тесты канона и отказов.

- [ ] **Шаг 5: коммит**

```bash
git add core/resolution.go core/mutate.go core/mutate_test.go
git commit -m "feat(core): MutPlaceKnown — рассказанная дорога через границу мутаций"
```

---

### Задача 5: полномочие роли актёра

**Файлы:**
- Изменить: `llm/capability.go:44-56` (`ProposalKind`), `llm/capability.go:83-120`
  (`Capabilities[RoleActor]`), `propose/propose.go:27-31` (`needs`)
- Тест: `propose/propose_test.go`, `llm/capability_test.go`

**Интерфейсы:**
- Потребляет: `core.MutPlaceKnown` из задачи 4.
- Отдаёт: `llm.ProposePlaceKnown ProposalKind = "place_known"`; запись
  `core.MutPlaceKnown → llm.ProposePlaceKnown` в `propose.needs`.

- [ ] **Шаг 1: написать падающий тест**

В `propose/propose_test.go`:

```go
// Рассказать дорогу вправе тот, кто говорит, — актёр. Это осознанное
// расширение его полномочий: до сих пор Proposes у него был пуст.
func TestActorMayTellThePlace(t *testing.T) {
	g := game() // n_quay, дело harbour
	g.DB.Locations["n_warehouse"] = store.Location{ID: "n_warehouse"}
	g.DB.Locations["n_quay"] = store.Location{ID: "n_quay",
		Adjacent: []store.NodeID{"n_warehouse"}}

	if _, ref := Mutation(g, llm.RoleActor,
		core.Mutation{Kind: core.MutPlaceKnown, Target: "n_warehouse"}); ref.Refused() {
		t.Errorf("актёр не смог рассказать дорогу: %q", ref.Reason)
	}
	if !g.KnowsPlace("n_warehouse") {
		t.Error("место не стало известным")
	}
}

// Место вправе рассказать ровно одна роль. Список закрыт нарочно: судья и
// модерация состояния не касаются вовсе, а Мастер говорит не за персонажа.
func TestOnlyActorMayTellPlaces(t *testing.T) {
	allowed := map[llm.Role]bool{llm.RoleActor: true}
	for _, role := range llm.AllRoles() {
		if got := Allowed(role, core.MutPlaceKnown); got != allowed[role] {
			t.Errorf("роль %q: место разрешено=%v, ждали %v", role, got, allowed[role])
		}
	}
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Запуск: `go test ./propose/`
Ожидание: FAIL — `Allowed(RoleActor, MutPlaceKnown)` возвращает false.

- [ ] **Шаг 3: реализовать**

В `llm/capability.go` к константам `ProposalKind`:

```go
	// ProposePlaceKnown — рассказать игроку о месте, куда отсюда ведёт дорога.
	// Не решение о мире: место в деле уже есть, персонаж лишь говорит, что оно
	// существует и где.
	ProposePlaceKnown ProposalKind = "place_known"
```

В `Capabilities[RoleActor]` добавить строку `Proposes`, заменив комментарий
«Актёр не предлагает ничего» на:

```go
	// Актёр предлагает ровно одно: место, о котором рассказал. Фактов дела и
	// решений о мире он по-прежнему не предлагает — чего не знает, спрашивает у
	// Мастера. Вид узкий, и выдумать место нельзя: набор узлов закрыт делом.
	RoleActor: {
		Reads: []ReadScope{ /* без изменений */ },
		Proposes: []ProposalKind{ProposePlaceKnown},
	},
```

В `propose/propose.go` в `needs`:

```go
	core.MutPlaceKnown: llm.ProposePlaceKnown,
```

- [ ] **Шаг 4: тесты зелёные**

Запуск: `go test ./propose/ ./llm/`
Ожидание: PASS. Если в `llm/capability_test.go` есть тест «актёр ничего не
предлагает» — переписать его на «актёр предлагает только место», сохранив
проверку, что фактов и мира он не предлагает.

- [ ] **Шаг 5: коммит**

```bash
git add llm/capability.go propose/propose.go propose/propose_test.go llm/capability_test.go
git commit -m "feat(propose): полномочие актёра рассказать о месте"
```

---

### Задача 6: актёр объявляет рассказанное место

**Файлы:**
- Изменить: `actor/actor.go` — `Situation` (поле `Roads`), `Reply` (поле
  `TellPlace`), `schemaFor`, разбор ответа в `speak`, `renderPrompt`,
  `GameVoicer.Voice`
- Тест: `actor/actor_test.go`

**Интерфейсы:**
- Потребляет: `propose.Mutation`, `core.MutPlaceKnown`, `llm.RoleActor`.
- Отдаёт: `Situation.Roads []Known` — смежные узлы (ID и название);
  `Reply.TellPlace string`.

- [ ] **Шаг 1: написать падающий тест**

```go
// Персонаж, рассказавший дорогу, делает место известным — через границу, а не
// сам. Живой прогон: Нильс подробно объяснил путь до склада, а игра туда не
// пустила, потому что рассказ ничего не менял.
func TestToldPlaceBecomesKnown(t *testing.T) {
	g := harbour(t)
	v, f := voicer(t, g, `{"line":"Прямо по настилу до второй тумбы.","told_place":"n_warehouse"}`)

	if _, err := v.Voice(context.Background(), core.Intent{Verb: "ask_about",
		Args: core.Args{Target: "e_nils", Text: "как дойти до склада?"}},
		core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if !g.KnowsPlace("n_warehouse") {
		t.Error("рассказанное место не стало известным")
	}
	// Перечисление в схеме — смежные узлы: назвать несмежное персонаж не может
	// физически, а не по обещанию в промпте.
	if in := f.Calls()[0].Schema; !strings.Contains(in, "n_warehouse") {
		t.Errorf("смежные места не попали в схему ответа:\n%s", in)
	}
}

// Несмежное место ядро отвергает, и ход от этого не рушится: реплика
// произносится, просто мир не меняется.
func TestToldPlaceOutsideReachIsRefusedQuietly(t *testing.T) {
	g := harbour(t)
	v, _ := voicer(t, g, `{"line":"За кузницей таверна.","told_place":"n_tavern"}`)

	got, err := v.Voice(context.Background(), core.Intent{Verb: "ask_about",
		Args: core.Args{Target: "e_nils", Text: "а где выпить?"}}, core.TurnResult{})
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Error("реплика потерялась из-за отвергнутого места")
	}
	if g.KnowsPlace("n_tavern") {
		t.Error("несмежное место стало известным")
	}
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Запуск: `go test ./actor/ -run ToldPlace`
Ожидание: FAIL — место не становится известным, `told_place` в схеме нет.

- [ ] **Шаг 3: реализовать**

`Situation` — новое поле рядом с `Scene`:

```go
	// Roads — места, куда отсюда ведёт дорога: ID и название. Материал не про
	// дело, а про географию, и она у всех на виду. Из него собирается
	// перечисление told_place: назвать несмежное место персонаж не может
	// физически.
	Roads []Known
```

`Reply` — новое поле:

```go
	// TellPlace — место, о котором персонаж рассказал. Схема, а не разбор
	// текста: догадываться по реплике, упомянул ли он склад, значит завести
	// второй ненадёжный парсер.
	TellPlace string
```

`schemaFor` получает `roads []Known` и добавляет свойство:

```go
			"told_place": enumOrEmptyPlaces(roads),
```

где рядом с `topicField`:

```go
// enumOrEmptyPlaces — перечисление мест, о которых персонаж вправе рассказать,
// плюс пустая строка. Пустая — обычный случай: дорогу объясняют не каждый ход.
func enumOrEmptyPlaces(roads []Known) map[string]any {
	ids := []string{""}
	for _, r := range roads {
		ids = append(ids, r.ID)
	}
	return map[string]any{"type": "string", "enum": ids,
		"description": "место, о котором ты рассказал, как туда попасть; пусто, если не рассказывал"}
}
```

Разбор в `speak` — к анонимной структуре добавить
`TellPlace string \`json:"told_place"\`` и перенести в `Reply`.

`renderPrompt` — после блока обстановки:

```go
	if len(sit.Roads) > 0 {
		b.WriteString("Куда отсюда ведёт дорога — об этом можно рассказывать:\n")
		for _, r := range sit.Roads {
			b.WriteString("  " + r.ID + " — " + r.Text + "\n")
		}
		b.WriteString("Рассказал, как туда попасть — поставь told_place.\n")
	}
```

`schemaFor` вызывается из `speak` через `schemaJSON` — обновить обе подписи:
`schemaJSON(sit.Talks, sit.Roads)` и `schemaFor(talks []Topic, roads []Known)`.

Объявленное место надо вынести из `Reply` наружу: `talk` возвращает только
строку, и `GameVoicer` в `Reply` не заглядывает. Шов — необязательный колбэк на
`Actor`, ровно как уже сделанный `notify`. Поле `told func(string)` в структуру
`Actor` рядом с `notify`, и:

```go
// WithTold включает сообщение о том, какое место персонаж назвал. Необязателен:
// без него рассказ дорогу не открывает, и игра работает как раньше.
func (a *Actor) WithTold(f func(place string)) *Actor {
	a.told = f
	return a
}
```

Вызов — в `finish`, сразу после разбора темы и ДО проверок реплики: место
персонаж назвал независимо от того, уцелеет ли реплика.

```go
	if out.TellPlace != "" && a.told != nil {
		a.told(out.TellPlace)
	}
```

`GameVoicer` ставит колбэк там же, где заполняет ситуацию:

```go
	sit.Roads = roadsFrom(v.Game)
	v.Actor = v.Actor.WithTold(func(place string) {
		// Предложение, а не запись: ядро проверит, что место существует и что
		// отсюда туда ведёт дорога. Отказ ход не рушит — реплика уже сказана.
		propose.Mutation(v.Game, llm.RoleActor, core.Mutation{
			Kind: core.MutPlaceKnown, Target: place,
		})
	})
```

и `roadsFrom`:

```go
// roadsFrom — куда отсюда ведёт дорога. Смежность здесь и остаётся: она решает
// не то, куда можно пойти, а то, о чём персонаж вправе рассказать.
func roadsFrom(g *core.Game) []Known {
	var out []Known
	for _, n := range g.DB.Locations[g.Node].Adjacent {
		out = append(out, Known{ID: string(n), Text: g.DB.Locations[n].Name})
	}
	return out
}
```

- [ ] **Шаг 4: тесты зелёные**

Запуск: `go test ./actor/`
Ожидание: PASS, включая существующие тесты реплик и заглушки.

- [ ] **Шаг 5: коммит**

```bash
git add actor/actor.go actor/actor_test.go
git commit -m "feat(actor): told_place — рассказанная дорога предлагается ядру"
```

---

### Задача 7: разбор — тема у `examine`/`search`

**Файлы:**
- Изменить: `intent/parser.go:372-380` (`takesTopic`), `intent/parser.go:56-70`
  (правило промпта)
- Тест: `intent/parser_test.go`

**Интерфейсы:** ничего нового не отдаёт.

- [ ] **Шаг 1: написать падающий тест**

```go
// «Искать конкретное» обязано быть выразимо: правило «назвал — знай» в ядре
// есть (validate отказывает на неизвестной теме), но парсер тему для осмотра
// не ставил, и свободным текстом такой ход не выражался вовсе.
func TestExamineTakesTopic(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"examine","target":"e_body",`+
		`"topic":"f_seal_cord","item":""}`)
	hint := harbourHint(t)
	hint.Topics = []Named{{ID: "f_seal_cord", Name: "гильдейский шнур"}}

	res, err := p.Parse(context.Background(), "осмотреть шнур на теле", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent == nil || res.Intent.Args.Topic != "f_seal_cord" {
		t.Errorf("тема осмотра потерялась: %+v", res.Intent)
	}
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Запуск: `go test ./intent/ -run TestExamineTakesTopic`
Ожидание: FAIL — тема сброшена правилом «аргумент, которого глагол не берёт».

- [ ] **Шаг 3: реализовать**

`takesTopic` (`intent/parser.go`):

```go
	case "question", "cross_reference", "examine", "search":
		return true
```

В комментарий функции добавить: `examine` и `search` тему принимают, потому что
«искать конкретное» — законный ход, а гейт знания стоит в ядре и срабатывает
сам. `ask_about` по-прежнему нет: вопрос о мире темы дела не имеет.

В правило 3 промпта добавить строку:

```
   - examine и search тему тоже принимают: «осмотреть шнур на теле» — это
     examine с темой. Тема только из списка известных; не знаешь — общий осмотр.
```

- [ ] **Шаг 4: тесты зелёные**

Запуск: `go test ./intent/`
Ожидание: PASS.

- [ ] **Шаг 5: коммит**

```bash
git add intent/parser.go intent/parser_test.go
git commit -m "feat(intent): тема у examine и search — «искать конкретное» выразимо"
```

---

### Задача 8: дело — поле `start_places`

**Файлы:**
- Изменить: `cases/schema.go:70-82` (поле), `cases/load.go:30-60` (перенос в
  `Config`), `cases/validate.go:211-232` (проверка), `cases/harbour/case.json`
- Тест: `cases/validate_test.go` (хелпер `mutate` уже есть), `cases/load_test.go`

**Интерфейсы:**
- Потребляет: `core.Config.StartPlaces` из задачи 2.
- Отдаёт: поле дела `"start_places": ["n_warehouse"]`.

- [ ] **Шаг 1: написать падающий тест**

```go
// Место, названное брифингом, обязано быть известно с начала: иначе игрок
// читает про склад в первой строке и не может туда пойти.
func TestStartPlacesReachTheGame(t *testing.T) {
	cfg, err := Load("harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, n := range cfg.StartPlaces {
		if n == "n_warehouse" {
			found = true
		}
	}
	if !found {
		t.Error("склад не объявлен известным с начала")
	}
}

// Опечатка в start_places — это место, которого нет: валидатор обязан сказать
// об этом при загрузке, а не оставить игрока без выхода.
func TestValidateRejectsUnknownStartPlace(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.StartPlaces = []store.NodeID{"n_нет"}
	})
	if err == nil {
		t.Error("несуществующее стартовое место принято")
	}
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Запуск: `go test ./cases/ -run "StartPlaces|UnknownStartPlace"`
Ожидание: не компилируется — `unknown field StartPlaces`.

- [ ] **Шаг 3: реализовать**

`cases/schema.go` рядом со `StartFacts`:

```go
	// StartPlaces — места, о которых игрок знает с начала: их назвал брифинг.
	// Без этого поля место, названное прозой, остаётся недостижимым, потому что
	// связь прозы с узлом машиночитаемой не бывает.
	StartPlaces []store.NodeID `json:"start_places"`
```

`cases/load.go` — в собираемый `core.Config`: `StartPlaces: f.StartPlaces`.

`cases/validate.go` после блока `FactUnlocks`:

```go
	for _, n := range f.StartPlaces {
		if !hasLocation(f, n) {
			add("стартовое место %s не существует", n)
		}
	}
```

`cases/harbour/case.json` — рядом со `"start_facts"`:

```json
  "start_places": ["n_warehouse"],
```

- [ ] **Шаг 4: тесты зелёные**

Запуск: `go test ./cases/`
Ожидание: PASS.

- [ ] **Шаг 5: коммит**

```bash
git add cases/schema.go cases/load.go cases/validate.go cases/harbour/case.json cases/validate_test.go cases/load_test.go
git commit -m "feat(cases): start_places — места, названные брифингом"
```

---

### Задача 9: e2e — решаемость на знании

**Файлы:**
- Изменить: `e2e/reachability_test.go:50-58` (`moveRandomly`), при
  необходимости `e2e/walkthrough_test.go`
- Тест: там же

**Интерфейсы:** ничего нового.

- [ ] **Шаг 1: прогнать и собрать факты**

Запуск: `go test ./e2e/ -run "Reachability|Walkthrough|Solvable" -v`
Записать, что упало и на каком деле. `moveRandomly` уже ходит по
`ReachableNodes()`, поэтому механически он работает — падение будет означать,
что дело стало непроходимым, и это ровно тот сигнал, ради которого тест есть.

- [ ] **Шаг 2: добавить тест на инвариант знания**

```go
// Каждое место дела обязано становиться известным: место, о котором нельзя
// узнать ни из брифинга, ни из факта, ни от соседа, не существует для игрока.
func TestEveryPlaceCanBecomeKnown(t *testing.T) {
	for name, path := range caseFiles {
		t.Run(name, func(t *testing.T) {
			cfg, err := cases.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			known := map[store.NodeID]bool{cfg.Start: true}
			for _, n := range cfg.StartPlaces {
				known[n] = true
			}
			for _, us := range cfg.DB.Unlocks {
				for _, u := range us {
					if u.UnlocksKind == "node" {
						known[store.NodeID(u.UnlocksID)] = true
					}
				}
			}
			// Смежное известному место рассказывает сосед — до тех пор, пока
			// множество растёт.
			for grew := true; grew; {
				grew = false
				for n := range known {
					for _, a := range cfg.DB.Locations[n].Adjacent {
						if !known[a] {
							known[a] = true
							grew = true
						}
					}
				}
			}
			for id := range cfg.DB.Locations {
				if !known[id] {
					t.Errorf("место %s недостижимо: о нём нельзя узнать никак", id)
				}
			}
		})
	}
}
```

- [ ] **Шаг 3: починить то, что упало**

Правки только в тестах либо в данных дел. Если непроходимым стало «Форте
Мерло» — смотреть, какое место осталось без пути к знанию, и объявлять его в
`start_places` того дела. Механику под тест не менять: инвариант §8.6 спеки
говорит именно о решаемости данных.

- [ ] **Шаг 4: полный прогон**

Запуск: `go test ./... && go vet ./... && gofmt -l .`
Ожидание: зелено, вывод `gofmt` пуст.

- [ ] **Шаг 5: коммит**

```bash
git add e2e/
git commit -m "test(e2e): решаемость дел на знании мест"
```

---

### Задача 10: убрать бессмысленный анлок склада

**Файлы:**
- Изменить: `cases/harbour/case.json` (`fact_unlocks`)

**Интерфейсы:** ничего.

- [ ] **Шаг 1: удалить запись**

Из `fact_unlocks` убрать:

```json
    { "fact_id": "f_ledger_erasure", "unlocks_kind": "node", "unlocks_id": "n_warehouse" }
```

Склад известен с начала (задача 8), поэтому запись безвредна, но врёт о
замысле: она читается как «гроссбух рассказывает про склад», а склад назван
брифингом.

- [ ] **Шаг 2: прогон**

Запуск: `go test ./...`
Ожидание: зелено. Если упало — значит на этой записи что-то держалось;
вернуть её и записать в отчёт, что именно.

- [ ] **Шаг 3: коммит**

```bash
git add cases/harbour/case.json
git commit -m "fix(harbour): убрать анлок склада — он известен из брифинга"
```

---

## Проверка после всех задач

- `go test ./...`, `go vet ./...`, `gofmt -l .` — чисто.
- Живая проба (нужен ключ; ~$0.05):

```bash
printf 'Обращаюсь к Нильсу и спрашиваю как дойти до склада\nИду к складу\nquit\n' > /tmp/p.txt
go run ./cmd/dnd --case cases/harbour/case.json --chat --plain --script /tmp/p.txt --journal /tmp/j.jsonl --provider openrouter --model anthropic/claude-sonnet-5 --model-cheap anthropic/claude-haiku-4.5 --price-in 2 --price-out 10 --price-in-cheap 1 --price-out-cheap 5 --cap-day 0.20
```

Ожидание: второй ход применён, а не отвергнут, и игрок оказывается на складе.
В журнале у обоих ходов `core_verdict: applied`.

- Обновить `docs/status.md`: §1 — знание мест вместо разрешения; §2 — новые
  инварианты (§8 спеки); §4 — охраняемые места отложены сознательно.

## Дальше (не в этой работе)

Охраняемые места («знаешь дорогу, но не пускают»); расстояние и время пути;
знание мест между парти в общем мире; `needs` в аудит-поток, чтобы «не спросил»
отличалось от «спросил, Мастер отказал».
