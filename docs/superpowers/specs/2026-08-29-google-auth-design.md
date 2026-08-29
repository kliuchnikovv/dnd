# Авторизация через Google (сервер + RN-клиент). Дизайн

Дата: 2026-08-29
Статус: согласован, готов к плану
Область: сервер (`github.com/kliuchnikovv/dnd`) + RN-клиент (`client/`)
Основание: транспорт nomi (`../genie-api`, `api/internal/auth`, `services/auth_service.go`), ADR-0004 (транспорт), серверная спека `2026-08-21-server-railway-design.md` («MVP — анонимный токен, Apple-auth позже»)

## 1. Повод

Сейчас бэкенд открыт: WS принимает любой непустой `token`, подписи не проверяет, сессии анонимны — кто угодно, зная `chat_id`, подключается к чужой игре. Это был осознанный MVP-выбор; пришло время поставить настоящую защиту. Делаем вход через Google-аккаунт по образцу nomi и экраны логина/регистрации в клиенте.

## 2. Решения (согласовано)

1. **Только Google** (без email/пароля). Первый вход = регистрация.
2. **Сессии приватные:** `sessions.user_id`, WS проверяет владение `chat_id`.
3. **С refresh-токенами** (как nomi): access 24ч + refresh 30д с ротацией.
4. **Анонимные сессии отсекаются** при включённом гейте.
5. **Гейт всегда строгий:** сервер без `JWT_SECRET` не стартует; локаль/тесты — через `DEV_AUTH`.
6. **A и B вместе**, одной спекой; `NetSource` (живой сокет клиента) — вне области.

## 3. Модель nomi (что повторяем)

Клиент делает Google Sign-In → получает Google **ID-token** → `POST /auth/google {id_token}` → сервер проверяет ID-token через OIDC (audience = Google client ID) → **находит-или-создаёт** пользователя → выдаёт **свой** JWT (HS256) + refresh. Клиент шлёт access как `token` в query WS и `Authorization: Bearer` в REST; на 401 — refresh. Плагинный OAuth nomi (Gmail/Calendar) — не про вход, не берём.

## 4. Инварианты

1. Личность проверяется криптографически: принимается только наш JWT, подписанный `JWT_SECRET`; Google ID-token проверяется против `GOOGLE_CLIENT_ID`.
2. Приватность сессий: доступ к `chat_id` только у владельца (`sessions.user_id`).
3. Граница слоёв: пакет `auth` живёт НАД доменом; `core`/`rules`/`store`/`dice`/`cases` о нём не знают; `e2e/architecture_test.go` зелёный.
4. Прод-лазеек нет: `POST /auth/dev` существует только при `DEV_AUTH=1`.
5. Секреты не в коде и не в репозитории: `JWT_SECRET`, `GOOGLE_CLIENT_ID` — из env. В БД хранится хеш refresh-токена, не сам токен.
6. `go test ./...` зелёный; `architecture_test` зелёный.

---

# A. Сервер

## A.1 API-контракт (стык A↔B)

- `POST /auth/google` `{ "id_token": "<google-id-token>" }`
  → `200 { "access_token", "refresh_token", "expires_at", "user": {"id","email","name","picture"} }`
  Ошибка проверки ID-token → `401`.
- `POST /auth/refresh` `{ "refresh_token" }`
  → `200 { "access_token", "refresh_token", "expires_at" }` (ротация: старый refresh гасится, выдаётся новый). Неизвестный/истёкший → `401`.
- `GET /me` (Bearer) → `200 { "id","email","name","picture" }`.
- `POST /auth/dev` `{ "email" }` → как `/auth/google`, но без Google. **Только при `DEV_AUTH=1`; иначе `404`.**
- Существующие: `POST /sessions` и `GET /chat/ws` — теперь под гейтом (см. A.4).

## A.2 Данные (embed-миграция `0002_auth.sql`)

```sql
CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,           -- uuid
    google_sub TEXT UNIQUE,                -- subject из Google (стабильный id)
    email      TEXT NOT NULL,
    name       TEXT,
    picture    TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    token_hash TEXT PRIMARY KEY,           -- sha256(refresh), сам токен не хранится
    user_id    TEXT NOT NULL REFERENCES users(id),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS user_id TEXT REFERENCES users(id);
```

Старые анонимные сессии (`user_id IS NULL`) при строгом гейте недоступны — доступ требует совпадения владельца.

## A.3 Пакет `auth` (над доменом)

- `auth.Verifier` — интерфейс проверки Google ID-token: `Verify(ctx, idToken) (Identity{Sub,Email,Name,Picture}, error)`. Прод-реализация — `google.golang.org/api/idtoken` (audience = `GOOGLE_CLIENT_ID`). Тесты — фейковый верификатор без сети.
- `auth.Tokens` — выдача/проверка/ротация: `Issue(user) (access, refresh, expiry)`, `Verify(access) (Claims, error)`, `Rotate(refresh) (...)`. JWT HS256 на `JWT_SECRET`; refresh — случайные 32 байта, в БД лежит `sha256`.
- `auth.Service` — `GoogleLogin(idToken)`, `Refresh(refresh)`, `Me(userID)`; find-or-create по `google_sub`. Хранилище — расширение серверного `Store` (users/refresh_tokens) рядом с журналом.
- Пакет импортирует `net/http`? Нет — HTTP-хендлеры остаются в `server`; `auth` даёт чистую логику. `server` зовёт `auth.Service`.

## A.4 Гейт (middleware в `server`)

- REST: `Authorization: Bearer <access>` → `authMiddleware` кладёт `userID` в контекст; нет/битый → `401`.
- WS: токен в `?token=` (как nomi) → тот же разбор до апгрейда; нет/битый → `401` до `websocket.Accept`.
- `POST /sessions`: создаёт сессию с `user_id` из токена.
- `GET /chat/ws`: после проверки токена — сверка, что `chat_id` принадлежит `userID` (иначе `403` до апгрейда). Реконнект-семантика фазы 5 не меняется.
- Строгий старт: без `JWT_SECRET` `cmd/server` падает с понятной ошибкой; `GOOGLE_CLIENT_ID` обязателен, если не задан только `DEV_AUTH`.

## A.5 Конфиг и зависимости

- Env: `JWT_SECRET` (обязателен), `GOOGLE_CLIENT_ID` (обязателен для реального входа), `DEV_AUTH` (по умолчанию выкл).
- Allowlist `architecture_test`: `google.golang.org/api/idtoken`, `github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`.

## A.6 Тестирование (сервер)

- `auth`: фейковый `Verifier` → `GoogleLogin` создаёт/находит пользователя; `Issue`/`Verify`/`Rotate` (ротация гасит старый refresh); истёкший/чужой refresh → ошибка.
- `server`: нет токена → `POST /sessions` и WS дают `401`; чужой `chat_id` → `403`; свой → ок и сессия под `user_id`; `POST /auth/dev` 404 без `DEV_AUTH`, ок с ним.
- pgStore: users/refresh_tokens CRUD (gated по `DATABASE_URL`, как фаза 6).
- Существующие серверные тесты обновляются: чеканят валидный access через `auth` (или `DEV_AUTH`), затем ходят как раньше.

## A.7 Фазы сервера

1. Пакет `auth`: `Verifier`(fake+google), `Tokens`(JWT+refresh), `Service`(find-or-create) — TDD, без сети.
2. Хранилище: миграция `0002`, users/refresh_tokens в mem- и pg-Store; `sessions.user_id`.
3. Хендлеры + middleware: `/auth/google`, `/auth/refresh`, `/me`, `/auth/dev`; гейт REST/WS; владение `chat_id`.
4. Строгий старт `cmd/server`; обновление существующих тестов; allowlist; e2e зелёный.

---

# B. RN-клиент (`client/`)

Клиент — тонкий рендерер turn-view на моках (`TurnViewSource`→`MockSource`). Экраны за `AppShell`. Добавляем вход, не трогая геймплей.

## B.1 Состав

- **Google Sign-In:** `expo-auth-session/providers/google` (Expo SDK 57) → Google **ID-token**. Клиентские Google-client-id (iOS/Android/web) — из конфига (`app.json`/env).
- **Хранение токенов:** `expo-secure-store` (не AsyncStorage) — access+refresh в защищённом хранилище.
- **`src/state/useAuth.ts`** (zustand): `{status:'loading'|'signedOut'|'signedIn', user, tokens}`, действия `login()`, `logout()`, `refresh()`, `restore()` (чтение из secure-store на старте).
- **`src/net/authClient.ts`:** `googleLogin(idToken)`, `refresh(token)`, `me()`; базовый URL сервера из конфига; на `401` в `me`/будущих вызовах — `refresh` и повтор.
- **Гейт оболочки:** `status==='signedIn'` → текущий `AppShell`; иначе → **экран входа** («Войти через Google»). Первый вход создаёт аккаунт; опц. лёгкий **экран профиля/онбординга** (показать имя из Google, «Продолжить»).
- Токен готов для будущего `NetSource`; сам `NetSource` — вне области.

## B.2 Экраны

- `src/screens/auth/SignInScreen.tsx` — логотип/слоган + кнопка «Войти через Google»; ошибки входа.
- `src/screens/auth/ProfileScreen.tsx` (опц.) — приветствие по имени из Google, аватар, «Продолжить». Заодно точка выхода (`logout`).

## B.3 Тестирование (клиент)

- `authClient` против мока сервера (успех/`401`/refresh-повтор).
- `useAuth` — редьюсеры/переходы статусов; `restore` из secure-store (мок).
- Гейт-рендер: `signedOut`→SignInScreen, `signedIn`→AppShell (без обращения к сети).

## B.4 Фазы клиента

1. `useAuth` + `authClient` + secure-store — TDD (моки).
2. `SignInScreen` + Google-провайдер; гейт в оболочке.
3. `ProfileScreen`/онбординг + `logout`.

---

## Границы (не здесь)

- Email/пароль; `NetSource` (живой сокет клиента ↔ сервер); мультиаккаунт и «выйти со всех устройств»; ротация `JWT_SECRET`; rate-limit на `/auth/*`; Apple-auth.

## Замечание по ветке

Серверный слой (`worktree-server-railway`) завершён и ждёт слияния в `main`. Эта фича — отдельный эпик; реализацию вести на свежей ветке от обновлённого `main` (после слияния), а не поверх server-railway.
