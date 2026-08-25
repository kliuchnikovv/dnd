# Капабилити-таблица ролей. План реализации для Claude Code

Дата: 2026-08-21
Дизайн: `docs/superpowers/specs/2026-08-21-role-capabilities-design.md`
ADR: `docs/adr/0001-layered-world-trusted-core.md`
Исполнитель: локальный Claude Code (Go 1.26, `go test ./...`)

## Как запускать

> Прочитай `docs/adr/0001-layered-world-trusted-core.md`,
> `docs/superpowers/specs/2026-08-21-role-capabilities-design.md` и этот план.
> Выполни **Фазу 1**, TDD. После фазы — `go test ./...`, коммить на зелёном.
> К следующей фазе — с моего слова.

## Инварианты (во всех фазах)

1. Реестр — единственная правда; `MutatesState`/`RequiresStrictOutput` выводятся
   из него.
2. Поведение существующих ролей не меняется: `intent_parser` по-прежнему требует
   строгого провайдера, `actor`/`canon_guard` — нет.
3. Граница домена: реестр в `llm/`, `core`/`rules`/`store`/`cases`/`dice` его не
   импортируют. `e2e/architecture_test.go` зелёный.
4. `go test ./...` зелёный после каждой фазы.

---

## Фаза 1 — Типы и реестр

Тронуть `llm/` (новый `capability.go`):
- `type ReadScope string` + константы: `ReadScenePublic`, `ReadPartyKnowledge`,
  `ReadCarriedItems`, `ReadOwnDossier`, `ReadWorldCanon`, `ReadAuthoredFlavour`,
  `ReadMechanicalOutcome`, `ReadCandidateLine`, `ReadPlayerInput`.
  **Намеренно нет** `Truth`/`AllHolders`.
- `type ProposalKind string` + `ProposeNothing`, `ProposeCommand`,
  `ProposeCanonAmbient`, `ProposeWorldMutation`.
- `type Capability struct { Reads []ReadScope; Proposes []ProposalKind }`
  (поля прямой мутации нет — конструктивно).
- `var Capabilities = map[Role]Capability{...}` по таблице §3 дизайна.

Тесты:
- полнота: у каждой `Role` (список фиксировать явно) есть запись; лишних нет;
- ни одна запись не содержит запрещённого scope (проверять по отсутствию
  соответствующих констант — их просто нет, тест стережёт, что не добавили).

---

## Фаза 2 — Вывод строгости и роутинг по реестру

Тронуть `llm/role.go`, `llm/router.go`:
- `func RequiresStrictOutput(r Role) bool` = `len(Capabilities[r].Proposes) > 0`
  и все они — предложения, применяемые к состоянию (пока все ненулевые таковы).
- `Role.MutatesState()` сделать тонкой обёрткой над `RequiresStrictOutput` (либо
  пометить deprecated и заменить вызовы). Единственный текущий вызывающий —
  `Router.filter`.
- `Router.filter`: фильтровать по `RequiresStrictOutput(role)` вместо
  `role.MutatesState()`. Поведение для `intent_parser` не меняется.

Тесты:
- `intent_parser` требует строгого провайдера (как сейчас); нестрогая цель
  отфильтрована → `ErrSchemaUnsafe` из `Gateway.Do`;
- `actor`/`canon_guard` строгого не требуют — нестрогая цель проходит;
- `narrator`/`environment_curator` требуют строгого (новое, из таблицы);
- вывод `RequiresStrictOutput` совпадает с поведением фильтра для всех ролей.

---

## Фаза 3 — Инварианты границы в architecture_test

Тронуть `e2e/architecture_test.go` (или соседний файл в пакете):
- тест «никто из LLM не мутирует напрямую»: пока API мутаций нет — зафиксировать
  как проверку, что `Capability` не имеет поля прямой мутации и что состояние
  меняет только `core` (заготовка: проверить, что пакеты ролей — `actor`,
  будущий `narrator`/`master` — не импортируют `store` на запись; минимально —
  что домен единственный мутатор, как сегодня).
- тест «реестр покрывает все роли» продублировать на уровне границы, чтобы
  добавленная без капабилити роль роняла CI.

---

## Фаза 4 — Привести существующих вызывающих к чтению по scope

Цель: код actor/narrator/parser читает ровно то, что объявлено в его scope, —
чтобы контракт §5 был не только на бумаге.

Тронуть `actor/`, `intent/` (и `master/`, если уже есть):
- у каждого места сборки промпта свериться с scope роли: актёр — только
  `party_knowledge` + своё досье + гранты + сцена (уже так, зафиксировать
  комментарием со ссылкой на scope); парсер — сцена + `party_knowledge` +
  инвентарь; убрать любое чтение вне scope, если найдётся.
- где дёшево — прогонять чтение через один аксессор на scope, чтобы будущий
  runtime-enforcement имел точку.

Тесты: промпт роли не содержит данных вне её scope (проверять по входу, как уже
делает тест парсера/актёра на присутствие нужного).

## Фаза 5 — Проверка

`go test ./...` зелёный; live-прогон `--nl` ведёт себя как раньше (формализация
не меняет геймплей). Диф читаемый: реестр + вывод строгости + тесты, без правок
поведения существующих ролей.

## Дальше (не в этой работе)

С этой таблицей на руках следующий естественный пункт ADR-0001 — **API мутаций
ядра**: закрытый набор типов мутаций + валидатор, единственный путь, которым
`ProposeCanonAmbient`/`ProposeWorldMutation` доходят до состояния. Тогда включится
и runtime-enforcement read-scope, и рабочие пути нарратора/куратора.
