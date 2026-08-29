# NetSource — живое подключение клиента к серверу. Дизайн

Дата: 2026-08-29
Статус: согласован, готов к плану
Область: RN-клиент (`client/`) + сервер (`github.com/kliuchnikovv/dnd`, Go)
Основание: серверный транспорт (`server/` WS Frame-протокол, `docs/superpowers/specs/2026-08-21-server-railway-design.md`), клиентский шов `client/src/turnview/source.ts` (`TurnViewSource`), авторизация (`2026-08-29-google-auth-design.md`)

## 1. Повод

Клиент рендерит turn-view на `MockSource` (фикстуры) и не ходит на сервер. Сервер умеет всё realtime (сессии, ход → `session_state`, стрим прозы, реконнект, приватные сессии под auth). Нужно заменить `MockSource` реальным `NetSource` за тем же интерфейсом `TurnViewSource`, чтобы экраны turn-view не менялись, плюс дать игроку выбрать/создать сессию.

## 2. Решения (согласовано)

1. **Экран выбора сессии** (не «всегда новая» и не «resume молча»): список сессий игрока + «Новая игра».
2. **Всё одним эпиком, три слайса:** `GET /sessions` (сервер) + NetSource-транспорт (клиент) + SessionSelectScreen (клиент).
3. **Реконнект — lean:** backoff + ping/pong + опора на серверный ре-сенд `session_state`+`history`. Без outbox/stall-watchdog (nomi ChatSocket не портируем).
4. **Жизненный цикл соединения — в контейнере** (SessionScreen), turn-view-экраны не меняются.
5. **WS-обёртка своя** под наш `Frame`, с инъекцией фабрики сокета для тестов.

## 3. Инварианты

1. Экраны turn-view не меняются: `NetSource` реализует `TurnViewSource` (`current`/`send`/`subscribe`) как `MockSource`.
2. Клиент не решает исход: `send` лишь шлёт интент; состояние приходит от сервера (`session_state`).
3. Приватность: WS и REST идут под Bearer-токеном из `useAuth`; сервер уже проверяет владение `chat_id`.
4. Секретов в коде нет; токен берётся из `useAuth`, не хранится в NetSource дольше запроса.
5. `npx jest` зелёный, `npx tsc --noEmit` чистый; `go test ./...` зелёный.

## 4. Поток

```
SignIn → SessionSelectScreen ──выбор chat_id──▶ SessionScreen (контейнер)
             │  GET /sessions (список)               │ владеет жизненным циклом NetSource
             │  POST /sessions (новая)               │ connect → спиннер → turn-view
             ▼                                        ▼
      sessionsClient (REST, Bearer)          NetSource : TurnViewSource
                                              │  WS /chat/ws?token=&chat_id=
                                              │  send(intent) → message-кадр; резолв на следующий session_state
                                              │  session_state → view; data-дельты → view.narration; done
                                              ▼
                       существующие turn-view экраны (useTurnView) — без изменений
```

---

# Слайс A — сервер `GET /sessions` (Go)

## A.1 Хранилище
- `SessionRecord` получает `CreatedAt time.Time`.
- `Store` получает `SessionsByUser(ctx, userID string) ([]SessionRecord, error)`:
  - memStore: фильтр `s.sessions` по `UserID` (пустой userID не матчит легаси-сессии с пустым владельцем — их в списке нет).
  - pgStore: `SELECT chat_id, case_id, seed, user_id, created_at FROM sessions WHERE user_id=$1 ORDER BY created_at DESC` (+ добавить `created_at` в SELECT/scan существующего `Sessions`, чтобы `CreatedAt` заполнялся и там).

## A.2 Хендлер
- `GET /sessions` под `requireAuth`: `userIDFrom(ctx)` → `SessionsByUser` → `200 [{chat_id, case_id, seed, created_at (unix seconds)}]`. Нет токена → 401.
- `POST /sessions` (создание) уже есть — «Новая игра» зовёт его; он уже кладёт `user_id` из токена.
- Регистрация: `GET /sessions` добавляется рядом с `POST /sessions`; оба под гейтом, когда `auth != nil`.

## A.3 Тесты (сервер)
- memStore `SessionsByUser` возвращает только сессии владельца; два пользователя изолированы; пустой владелец не попадает.
- Хендлер: 200 со списком под токеном; 401 без токена; чужие сессии не видны.
- pgStore `SessionsByUser` — gated по `DATABASE_URL` (как фаза 6), проверяется на живом Postgres контроллером.

---

# Слайс B — NetSource транспорт (клиент, ядро)

## B.1 Frame (провод)
`client/src/net/frame.ts` — тип `Frame { id:number; chat_id:string; channel:string; kind:string; op:string; payload?:any; error?:any }` и константы op/kind/channel, зеркало сервера (`server/frame.go`). Плюс типы payload: `inputPayload {token?,text?}`, `textDelta {type:'text',delta,ix}`, `historyPayload {type:'narration',text}`.

## B.2 NetSource
`client/src/net/netSource.ts` — `class NetSource implements TurnViewSource`:
- Конструктор: `{ chatId, wsUrl, getToken(): Promise<string>, onAuthLost(): void, makeSocket?: (url) => WebSocketLike }`. `makeSocket` по умолчанию — глобальный `WebSocket`; в тестах инжектится фейк.
- `current(): TurnView` — внутренний `view`, стартует плейсхолдером `{ version: 0, scene: {…пусто}, options: [] }` (контейнер по `version===0` показывает спиннер).
- `connect()`: строит `${wsUrl}?token=${await getToken()}&chat_id=${chatId}`, открывает сокет. Обработка кадров:
  - `op:session_state` (kind:data) → заменить механику view (scene/options/meters/resolution/… из payload как `TurnView`), сбросить накопитель прозы нового хода; `emit`; резолвнуть ожидающий `send`.
  - `op:message` kind:data payload `{type:'text',delta,ix}` → дописать `delta` в `view.narration` (аккумулятор прозы хода); `emit`.
  - `op:done` → пометить прозу завершённой; `emit`.
  - `op:history` payload `{type:'narration',text}` → заполнить `view.narration` готовым текстом (реконнект завершённого хода); `emit`.
  - `op:ping` → ответить `ping`-кадром (pong).
  - `kind:error` → `emit` состояние ошибки / колбэк (не фатально).
- `send(intent: Intent): Promise<TurnView>` — отправить `op:message` `{token}` или `{text}` с монотонным `Frame.id` (клиентский счётчик, переживает реконнект — идемпотентность на сервере по id); вернуть Promise, резолвящийся СЛЕДУЮЩИМ `session_state`. Один ожидающий резолвер на ход (ходы последовательны). Таймаут/дисконнект → reject.
- `subscribe(listener)`: `Set` слушателей; `emit` на каждое изменение view; возвращает отписку.
- Реконнект: неожиданное закрытие → экспоненциальный backoff (1s→30s) → `connect()`; сервер сам ре-сендит `session_state`+`history`. `close()` — намеренная остановка (флаг, глушит реконнект).
- Auth: отказ на хендшейке/`kind:error code 401` → один раз `getToken` с рефрешем (через useAuth) → реконнект; повторный отказ → `onAuthLost()` (контейнер делает `logout()`).

## B.3 sessionsClient
`client/src/net/sessionsClient.ts` — `listSessions(token): Promise<SessionSummary[]>`, `createSession(token, caseName, seed?): Promise<{chatId}>` (fetch + `Authorization: Bearer`, маппинг snake→camel, как authClient). `SessionSummary { chatId, caseId, seed, createdAt }`.

## B.4 Тесты (NetSource)
Инъекция фейкового сокета (эмулятор `WebSocketLike`: open/message/close + `push(frame)`):
- после `session_state`: `current()`/`subscribe` дают вид с механикой;
- `data`-дельты копятся в `view.narration`, `done` завершает;
- `send(token)` резолвится на следующем `session_state`;
- реконнект: закрытие → повторный connect → новый `session_state` заменяет вид;
- `401` → `getToken` вызван для рефреша → реконнект; повторный `401` → `onAuthLost` вызван;
- `history` на реконнекте заполняет прозу.
sessionsClient — fetch-mock (list/create/401).

---

# Слайс C — SessionSelectScreen + проводка (клиент)

## C.1 Экран выбора
`client/src/screens/session/SessionSelectScreen.tsx`: на маунте `listSessions(token)` → список (дело + относительное время `createdAt`) + кнопка «Новая игра» (→ `createSession('harbour')`). Выбор/создание задаёт текущий `chatId` и ведёт в игру. Спиннер на загрузке списка, строка ошибки при сбое.

## C.2 Контейнер игры
`client/src/screens/session/SessionScreen.tsx`: строит `new NetSource({ chatId, wsUrl, getToken: () => useAuth token, onAuthLost: () => useAuth.logout() })`, `connect()` на маунте, `close()` на размонтировании. Пока `view.version===0` — спиннер; при обрыве — ненавязчивый баннер «переподключение»; иначе рендерит существующий turn-view через `useTurnView(netSource)` (экраны не меняются).

## C.3 Проводка в приложении
- App-гейт: `signedIn` → SessionSelectScreen → (выбран chatId) → SessionScreen. Небольшой стейт «текущий chatId» (zustand или локально в оболочке).
- `client/src/state/store.ts`: поле `source` переиспользуется под интерфейс `TurnViewSource` (сейчас жёстко `MockSource`); `MockSource` остаётся доступным как dev/offline-переключатель, боевой путь — NetSource из контейнера.

## C.4 Тесты (клиент C)
- sessionsClient — см. B.4.
- Логика выбора (маппинг список→вьюмодель, «новая игра» → createSession) — юнит на чистых функциях/редьюсерах.
- Рендер RN-экранов и реальный сокет — на устройстве.

---

## 5. Обработка ошибок

- Подключение — спиннер; обрыв — авто-реконнект + баннер, серверный ре-сенд восстанавливает состояние.
- Потеря auth — `onAuthLost` → `logout()` → SignIn.
- Невалидный интент — серверный `error`-кадр → неблокирующий тост, вид не меняется.
- `send` при разрыве — reject (lean, без outbox); UI разблокирует ввод.

## 6. Границы (не здесь)

Свободный NL-текст (на сервере нет парсера — клиент шлёт `{token}`/номер); мультиплеер и канал `user`; offline-очередь/outbox; оптимистичный UI; удаление/переименование сессий; выбор дела кроме harbour в «Новой игре».

## 7. Замечание по ветке

Реализацию вести на свежей ветке от `main` (обе половины auth уже в `main`). Три слайса — независимо ревьюируемые: сервер (Go, gated pg), транспорт (клиент, fake-socket), экран (клиент).
