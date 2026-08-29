# dnd server

Тонкий транспорт над доменом: HTTP для health и старта сессии, WebSocket для
реального времени (turn-view, стрим прозы). Игровой логики здесь нет —
состояние живёт в ядре, сервер лишь разворачивает интент, зовёт `core.Apply`,
строит `view.TurnView` и стримит прозу Мастера.

## Протокол

- `GET /healthz` → `200 {"status":"ok"}` — healthcheck.
- `POST /sessions` `{"case":"harbour","seed":1}` → `{"chat_id":"..."}` — старт сессии.
- `GET /chat/ws?token=<t>&chat_id=<id>` — WebSocket, кадр `Frame`, совместимый с
  клиентом nomi. Ход: `op:"message"` `{token}` или `{text:"N"}` →
  `session_state` (полный TurnView) → дельты прозы (`op:"message"`,
  `kind:"data"`, `{type:"text",delta,ix}`) → `op:"done"`.

Реконнект: сервер сразу отдаёт `session_state`, догоняет прозу в полёте
дельтами (или отдаёт `history` завершённого хода) и доигрывает остаток живой
рассылкой. Повторная доставка одного `Frame.id` ход дважды не применяет.

## Переменные окружения

| Переменная | Назначение | По умолчанию |
|---|---|---|
| `PORT` | порт HTTP/WS | `8080` |
| `CASES_DIR` | корень дел | `cases` (в образе `/app/cases`) |
| `DATABASE_URL` | Postgres для журнала; без него журнал в памяти | — |
| `MODEL` | модель прозы; без неё сервер отдаёт только механику | — |
| `PROVIDER` | `openrouter` \| `anthropic` | `openrouter` |
| `MODEL_CHEAP` | модель дешёвого тира | — |
| `CAP_DAY` | суточный потолок расхода, $ | `1.0` |
| `CAP_TURN` | потолок вызовов модели на ход | `8` |
| `PRICE_IN` / `PRICE_OUT` | цена модели, $/млн токенов, если её нет в таблице | — |
| `PRICE_IN_CHEAP` / `PRICE_OUT_CHEAP` | то же для дешёвой модели | — |
| `GUARD_LINES` | проверять прозу гвардом на утечку/противоречие | `true` |
| `OPENROUTER_API_KEY` / `ANTHROPIC_API_KEY` | ключ провайдера | — |

## Локально

```bash
# только механика (без модели), журнал в памяти
go run ./cmd/server

# с прозой и Postgres
DATABASE_URL=postgres://user:pass@localhost:5432/dnd \
MODEL=anthropic/claude-sonnet-4.5 OPENROUTER_API_KEY=... \
go run ./cmd/server
```

Проверка:

```bash
curl localhost:8080/healthz
curl -X POST localhost:8080/sessions -d '{"case":"harbour","seed":1}'
```

## Docker

```bash
docker build -t dnd-server .
docker run -p 8080:8080 -e MODEL=... -e OPENROUTER_API_KEY=... dnd-server
```

Образ: сборка на `golang:1.26`, рантайм — distroless (статический бинарь,
миграции вшиты, дела в `/app/cases`).

## Railway

1. Создать проект и привязать репозиторий (Railway подхватит `railway.toml` и
   `Dockerfile`), либо `railway init` + `railway up` из этого каталога.
2. Добавить плагин **Postgres** — он выставит `DATABASE_URL`. Миграции
   накатываются на старте (идемпотентны).
3. Задать переменные: `MODEL`, `PROVIDER`, ключ провайдера, `CAP_DAY`. `PORT`
   Railway проставляет сам.
4. Деплой: `railway up`. Healthcheck `/healthz` должен позеленеть.
5. Клиент цепляется живым сокетом на `wss://<домен>/chat/ws?token=...&chat_id=...`.

WebSocket идёт через прокси Railway: клиент держит `ping` против idle-таймаутов
(это делает `ChatSocket` nomi), сервер отвечает на `ping` и переживает
реконнект.
