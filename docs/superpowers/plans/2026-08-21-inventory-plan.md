# Инвентарь и предъявление. План реализации для Claude Code

Дата: 2026-08-21
Дизайн: `docs/superpowers/specs/2026-08-21-inventory-and-present-design.md`
Исполнитель: локальный Claude Code (Go 1.26, `go test ./...`)

## Как запускать

> Прочитай `docs/superpowers/specs/2026-08-21-inventory-and-present-design.md` и
> этот план. Выполни **Фазу 1**. TDD, тест раньше кода. После фазы —
> `go test ./...`, коммить только на зелёном. К следующей фазе не переходи без
> моего слова.

## Инварианты (во всех фазах)

1. Инвентарь и `present` — чистый домен (`core`/`store`), детерминизм. Озвучка
   реакции — над доменом. `e2e/architecture_test.go` остаётся зелёным.
2. `cases.truth` предмету и `present` недоступен.
3. Путь `mandatory` к каждому факту сохраняется; `e2e/reachability` и «40 seed
   из 40» зелёные.
4. Любой предмет, на который ссылается гейт, обязан быть добываемым — новый
   инвариант валидатора, падает на старте.
5. `go test ./...` зелёный после каждой фазы.

---

## Фаза 1 — Предмет и инвентарь (данные, без эффектов)

Цель: предметы грузятся, парти их несёт. Поведения ещё нет.

Тронуть:
- `store/rows.go`: тип `Item{ ID ItemID; CaseID CaseID; Kind string; Name, Text
  string; AssertsFact FactID; Tags []string }`; тип `ItemID string` (в
  `store/ids.go`).
- `store/db.go`: `Items map[ItemID]Item`; множество инвентаря парти
  `Inventory map[ItemID]bool` (или `map[PartyID]map[ItemID]bool` под будущее);
  методы `HasItem`, `AddItem`.
- `cases/schema.go`: `Items []store.Item` и `StartInventory []store.ItemID` в
  `File`.
- `cases/load.go`: разложить `items` в `DB.Items`, `start_inventory` в инвентарь.
- `core/game.go`: `Carries(ItemID) bool`, `Acquire(ItemID)`.

Тесты: загрузка предметов и старт-инвентаря; `Carries`/`Acquire`; отсутствие
предмета. Существующие кейс-тесты зелёные.

---

## Фаза 2 — Глагол `present` и гейт `requires_items`

Цель: детерминированный эффект предъявления.

Тронуть:
- `core/verbs.go`: `"present": {"present", ClassSocial, false, false, false}`.
  (Обновить `allverbs_test`/счётчики глаголов — их проверяет table-driven тест.)
- `store/rows.go`: `Gate.RequiresItems []ItemID`.
- `core/`: флаг предъявления в отношенческом слое — `Dossiers.MarkPresented(holder,
  item)`, `Presented(holder, item) bool` (хранить в `store.Dossier`, новое поле
  `Presented []ItemID`).
- `core/turn.go` `Apply`: ветка `present`:
  - валидация: несёт ли парти `item`; цель в сцене.
  - `MarkPresented(target, item)`; если у `item.Kind == credential` — авторский
    `Adjust` (величину взять из данных предмета/гейта; для MVP — фикс. +1 или
    поле `Item` `DispositionDelta`).
  - выдача факта: если держатель `target` держит факт, чей `gate.verbs` включает
    `present` и `gate.requires_items` удовлетворён предъявленным (и прочие
    `requires`), — открыть факт **без броска** (ветка mandatory-типа), записать в
    `party_knowledge`, применить `fact_unlocks`.
  - `spendTime`/`noteTurn` как у прочих ходов.
- `core/…` где вычисляется применимость гейта: учесть `RequiresItems` через
  `Presented`/`Carries` (решить владение-vs-предъявление по дизайну §5 — MVP:
  предъявление).

Тесты (table-driven):
- предъявил годный credential → факт открыт, диспозиция сдвинута;
- предъявил не тот предмет → факт закрыт, NPC не выдал (отказа-подсказки нет);
- `present` без предмета в инвентаре → отказ (не провал, ход не потрачен);
- повторное предъявление идемпотентно по диспозиции (не копит бесконечно —
  решить: один сдвиг на предмет, как `MarkTold`).

---

## Фаза 3 — Проводка парсера (снимает исходный баг «показать предписание»)

Тронуть:
- `intent/…` `SceneHint`: влить носимый инвентарь в `Tools` (или новое `Carried`),
  чтобы `item`-enum и `verbNamesFor` включали `present`. `BuildHint` берёт
  инвентарь из `core.Game`.
- `intent/arity.go`: у `present` обязателен `item`, опц. `target`.
- `intent/parser.go` `validate`: `resolveByName(text, carried)` для `item`, как
  для целей/тем; «предписание» → `i_writ`.
- Пример в `systemPrompt`: «показать предписание Берну» →
  `{"outcome":"intent","verb":"present","item":"i_writ","target":"e_bern"}`.

Тесты: с `llm/fake.go` — «показать предписание» разбирается в `present` с верным
`item`; без инвентаря `present` не в грамматике; имя предмета разрешается.

---

## Фаза 4 — Выдача предмета фактом и валидатор добываемости

Тронуть:
- `store`/`cases`: связь `fact → grants_item` (поле у `Fact` или отдельная
  таблица `fact_grants`). При `Learn(fact)` в `core` — `Acquire(grantedItem)`.
- `cases/validate.go`: каждый `ItemID` из любого `gate.requires_items` обязан
  быть в `start_inventory` **или** выдаваться каким-то фактом; иначе падение на
  старте с внятным сообщением.

Тесты: `validate_test` — недобываемый предмет в гейте роняет загрузку; выдача
предмета фактом кладёт его в инвентарь.

---

## Фаза 5 — Кейс «Гавань»: нить предписания + презентация

Тронуть:
- `cases/harbour/case.json`: предмет `i_writ` («Предписание магистрата»),
  `start_inventory: ["i_writ"]`; гейт кооперативного факта Берна —
  `verbs:["present","question"]`, `requires_items:["i_writ"]`, сдвиг диспозиции;
  сохранить `mandatory`-источник (Нильс).
  Флейвор: `present.e_bern` (что видно при предъявлении).
- `cli/render.go`: команда `items` (что несёшь) + строка в `help`; при `-nl`
  реакцию озвучивает актёр/мастер.
- `cases/harbour/walkthrough.txt`: добавить ветку через `present i_writ e_bern`;
  e2e-тест прохождения обновить — дело по-прежнему проходится и через предмет, и
  без него (через Нильса).

Тесты: e2e «Гавань» проходится обоими путями; распределение/достижимость
зелёные.

## Фаза 6 — Живой прогон

Сыграть `--nl`, проверить: «показать предписание» → `present`, Берн реагирует
живой репликой, факт открывается, без предписания — уклонение без подсказки.

## Чистка (после приёмки)

Свернуть тег `tool` пропа в общий вид предмета `kind:"tool"` отдельным дифом,
когда инвентарь принят, чтобы не держать две модели инструмента.
