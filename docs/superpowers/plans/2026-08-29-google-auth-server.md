# Google-авторизация: сервер (A). План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Закрыть бэкенд: вход через Google (ID-token → свой JWT+refresh), приватные сессии (владение `chat_id`), строгий гейт REST/WS.

**Architecture:** Пакет `auth` над доменом даёт чистую логику (проверка Google ID-token, выдача/ротация JWT, find-or-create пользователя). HTTP-хендлеры и middleware живут в `server` и зовут `auth.Service`. Пользователи и refresh-токены хранятся рядом с журналом (mem- и pg-Store); сессии получают `user_id`.

**Tech Stack:** Go 1.26, `google.golang.org/api/idtoken` (проверка Google ID-token), `github.com/golang-jwt/jwt/v5` (наш JWT), `github.com/google/uuid`, `coder/websocket`, `pgx/v5`, stdlib `net/http`.

**Spec:** `docs/superpowers/specs/2026-08-29-google-auth-design.md`

## Global Constraints

- Пакет `auth` НЕ импортирует `net/http` и живёт вне `pureLayers`; `core`/`rules`/`store`/`dice`/`cases` его не импортируют. `e2e/architecture_test.go` зелёный.
- Новые прямые зависимости добавляются в allowlist `TestDependenciesAreAllowlisted` (`e2e/architecture_test.go`): `google.golang.org/api/idtoken`, `github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`.
- Секреты только из env: `JWT_SECRET` (обязателен для старта), `GOOGLE_CLIENT_ID`, `DEV_AUTH`. Ни ключей, ни токенов в коде/репозитории. В БД — `sha256` refresh-токена, не сам токен.
- `go test ./...` зелёный; `go build ./cmd/server` собирается; `-race` чист на `server` и `auth`.
- Access-JWT: HS256, TTL 24ч. Refresh: 32 случайных байта, TTL 30д, ротация при обмене.
- Прод-лазеек нет: `POST /auth/dev` отвечает `404`, если `DEV_AUTH != "1"`.

---

## File Structure

- `auth/token.go` — выдача/проверка access-JWT, генерация refresh + его хеш. (Create)
- `auth/verifier.go` — интерфейс `Verifier` + прод-реализация на `idtoken` + `fakeVerifier` для тестов. (Create)
- `auth/service.go` — `Service`: `GoogleLogin`, `Refresh`, `Me`; интерфейс хранилища `UserStore`. (Create)
- `auth/*_test.go` — тесты пакета. (Create)
- `server/store.go` — расширить `Store` методами users/refresh; `memStore` реализует. (Modify)
- `server/migrations/0002_auth.sql` — `users`, `refresh_tokens`, `sessions.user_id`. (Create)
- `server/pgstore.go` — pg-реализация новых методов. (Modify)
- `server/manager.go` — `SessionRecord.UserID`; `Create(caseName, seed, userID)`; проверка владения. (Modify)
- `server/authhttp.go` — хендлеры `/auth/google`, `/auth/refresh`, `/me`, `/auth/dev`; middleware; извлечение токена. (Create)
- `server/server.go` — регистрация маршрутов + обёртка гейта на `/sessions` и `/chat/ws`. (Modify)
- `server/ws.go` — брать `userID` из токена, проверять владение `chat_id`. (Modify)
- `cmd/server/main.go` — строгий старт (`JWT_SECRET`), сборка `auth.Service`, `DEV_AUTH`. (Modify)
- `e2e/architecture_test.go` — allowlist + (при желании) проверка, что `auth` вне pureLayers. (Modify)

---

## Task 1: Пакет `auth` — токены (access-JWT + refresh)

**Files:**
- Create: `auth/token.go`
- Test: `auth/token_test.go`

**Interfaces:**
- Consumes: —
- Produces:
  - `type Identity struct { Sub, Email, Name, Picture string }`
  - `type Claims struct { UserID string }`
  - `type Tokens struct { ... }`
  - `func NewTokens(secret string, now func() time.Time) *Tokens`
  - `func (t *Tokens) IssueAccess(userID string) (token string, expiresAt time.Time, err error)`
  - `func (t *Tokens) VerifyAccess(token string) (userID string, err error)` — `ErrExpired`/`ErrInvalid`
  - `func (t *Tokens) NewRefresh() (raw string, hash string, expiresAt time.Time, err error)`
  - `func HashRefresh(raw string) string`
  - `var ErrExpired, ErrInvalid error`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"testing"
	"time"
)

func fixedNow(s string) func() time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return func() time.Time { return t }
}

func TestAccessRoundTrip(t *testing.T) {
	tk := NewTokens("secret", fixedNow("2026-08-29T00:00:00Z"))
	token, exp, err := tk.IssueAccess("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !exp.After(fixedNow("2026-08-29T00:00:00Z")()) {
		t.Fatalf("срок не в будущем: %v", exp)
	}
	uid, err := tk.VerifyAccess(token)
	if err != nil || uid != "user-1" {
		t.Fatalf("verify: uid=%q err=%v", uid, err)
	}
}

func TestAccessRejectsWrongSecret(t *testing.T) {
	a := NewTokens("secret-a", fixedNow("2026-08-29T00:00:00Z"))
	b := NewTokens("secret-b", fixedNow("2026-08-29T00:00:00Z"))
	token, _, _ := a.IssueAccess("u")
	if _, err := b.VerifyAccess(token); err == nil {
		t.Fatal("чужой секрет принят")
	}
}

func TestAccessExpires(t *testing.T) {
	tk := NewTokens("s", fixedNow("2026-08-29T00:00:00Z"))
	token, _, _ := tk.IssueAccess("u")
	late := NewTokens("s", fixedNow("2026-10-01T00:00:00Z"))
	if _, err := late.VerifyAccess(token); err != ErrExpired {
		t.Fatalf("ждали ErrExpired, получили %v", err)
	}
}

func TestRefreshHashStable(t *testing.T) {
	tk := NewTokens("s", fixedNow("2026-08-29T00:00:00Z"))
	raw, hash, exp, err := tk.NewRefresh()
	if err != nil || raw == "" {
		t.Fatalf("refresh: %v", err)
	}
	if HashRefresh(raw) != hash {
		t.Fatal("хеш refresh нестабилен")
	}
	if raw == hash {
		t.Fatal("в БД должен лежать хеш, не сам токен")
	}
	if !exp.After(fixedNow("2026-08-29T00:00:00Z")()) {
		t.Fatal("refresh истекает не в будущем")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./auth/ -run TestAccess -v`
Expected: FAIL — `undefined: NewTokens`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package auth — вход и удостоверение личности. Живёт НАД доменом: core и
// прочие чистые слои о нём не знают (см. e2e/architecture_test.go).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrExpired = errors.New("auth: токен истёк")
	ErrInvalid = errors.New("auth: токен недействителен")
)

const (
	accessTTL  = 24 * time.Hour
	refreshTTL = 30 * 24 * time.Hour
)

type Identity struct{ Sub, Email, Name, Picture string }

type claims struct {
	jwt.RegisteredClaims
}

type Tokens struct {
	secret []byte
	now    func() time.Time
}

func NewTokens(secret string, now func() time.Time) *Tokens {
	if now == nil {
		now = time.Now
	}
	return &Tokens{secret: []byte(secret), now: now}
}

func (t *Tokens) IssueAccess(userID string) (string, time.Time, error) {
	exp := t.now().Add(accessTTL)
	c := claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(t.now()),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	return tok, exp, err
}

func (t *Tokens) VerifyAccess(token string) (string, error) {
	c := &claims{}
	_, err := jwt.ParseWithClaims(token, c, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalid
		}
		return t.secret, nil
	}, jwt.WithTimeFunc(t.now))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrExpired
		}
		return "", ErrInvalid
	}
	return c.Subject, nil
}

func (t *Tokens) NewRefresh() (string, string, time.Time, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", time.Time{}, err
	}
	raw := hex.EncodeToString(b[:])
	return raw, HashRefresh(raw), t.now().Add(refreshTTL), nil
}

func HashRefresh(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./auth/ -v`
Expected: PASS (add the dep first if needed: `go get github.com/golang-jwt/jwt/v5`).

- [ ] **Step 5: Commit**

```bash
git add auth/token.go auth/token_test.go go.mod go.sum
git commit -m "feat(auth): access-JWT и refresh с хешированием"
```

---

## Task 2: Пакет `auth` — Verifier (Google ID-token)

**Files:**
- Create: `auth/verifier.go`
- Test: `auth/verifier_test.go`

**Interfaces:**
- Consumes: `Identity` (Task 1).
- Produces:
  - `type Verifier interface { Verify(ctx context.Context, idToken string) (Identity, error) }`
  - `func NewGoogleVerifier(ctx context.Context, clientID string) (Verifier, error)`
  - `type FakeVerifier struct { ID Identity; Err error }` + `Verify` (для тестов и `DEV_AUTH`).

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"context"
	"errors"
	"testing"
)

func TestFakeVerifier(t *testing.T) {
	var v Verifier = &FakeVerifier{ID: Identity{Sub: "g-1", Email: "a@b.c", Name: "Ann"}}
	id, err := v.Verify(context.Background(), "any")
	if err != nil || id.Sub != "g-1" || id.Email != "a@b.c" {
		t.Fatalf("fake verify: %+v err=%v", id, err)
	}
	bad := &FakeVerifier{Err: errors.New("boom")}
	if _, err := bad.Verify(context.Background(), "x"); err == nil {
		t.Fatal("ждали ошибку от fake")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./auth/ -run TestFakeVerifier -v`
Expected: FAIL — `undefined: Verifier`.

- [ ] **Step 3: Write minimal implementation**

```go
package auth

import (
	"context"
	"fmt"

	"google.golang.org/api/idtoken"
)

// Verifier проверяет Google ID-token и извлекает личность. Интерфейс — чтобы
// тесты и DEV_AUTH не ходили в сеть.
type Verifier interface {
	Verify(ctx context.Context, idToken string) (Identity, error)
}

type googleVerifier struct {
	validator *idtoken.Validator
	clientID  string
}

// NewGoogleVerifier — прод-проверка против accounts.google.com с audience=clientID.
func NewGoogleVerifier(ctx context.Context, clientID string) (Verifier, error) {
	v, err := idtoken.NewValidator(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: idtoken validator: %w", err)
	}
	return &googleVerifier{validator: v, clientID: clientID}, nil
}

func (g *googleVerifier) Verify(ctx context.Context, idToken string) (Identity, error) {
	p, err := g.validator.Validate(ctx, idToken, g.clientID)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: проверка Google ID-token: %w", err)
	}
	return Identity{
		Sub:     p.Subject,
		Email:   str(p.Claims["email"]),
		Name:    str(p.Claims["name"]),
		Picture: str(p.Claims["picture"]),
	}, nil
}

func str(v any) string { s, _ := v.(string); return s }

// FakeVerifier — детерминированная проверка для тестов и DEV_AUTH.
type FakeVerifier struct {
	ID  Identity
	Err error
}

func (f *FakeVerifier) Verify(context.Context, string) (Identity, error) {
	if f.Err != nil {
		return Identity{}, f.Err
	}
	return f.ID, nil
}
```

- [ ] **Step 4: Run tests + build**

Run: `go get google.golang.org/api/idtoken && go test ./auth/ -v && go build ./auth/`
Expected: PASS; сборка ок.

- [ ] **Step 5: Commit**

```bash
git add auth/verifier.go auth/verifier_test.go go.mod go.sum
git commit -m "feat(auth): Verifier Google ID-token + fake"
```

---

## Task 3: Хранилище пользователей и refresh-токенов (memStore)

**Files:**
- Modify: `server/store.go`
- Test: `server/store_test.go` (добавить)

**Interfaces:**
- Consumes: —
- Produces (в пакете `server`):
  - `type UserRecord struct { ID, GoogleSub, Email, Name, Picture string }`
  - расширение `Store`:
    - `UpsertUser(ctx, UserRecord) error`
    - `UserByGoogleSub(ctx, sub string) (UserRecord, bool, error)`
    - `UserByID(ctx, id string) (UserRecord, bool, error)`
    - `SaveRefresh(ctx, hash, userID string, expiresAt time.Time) error`
    - `RefreshOwner(ctx, hash string) (userID string, ok bool, err error)` — только не истёкший
    - `DeleteRefresh(ctx, hash string) error`
  - `SessionRecord` получает поле `UserID string`.

- [ ] **Step 1: Write the failing test**

```go
func TestMemStoreUsersAndRefresh(t *testing.T) {
	st := NewMemStore()
	ctx := context.Background()
	u := UserRecord{ID: "u1", GoogleSub: "g1", Email: "a@b.c", Name: "Ann"}
	if err := st.UpsertUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := st.UserByGoogleSub(ctx, "g1")
	if !ok || got.ID != "u1" {
		t.Fatalf("по sub: %+v ok=%v", got, ok)
	}
	byID, ok, _ := st.UserByID(ctx, "u1")
	if !ok || byID.Email != "a@b.c" {
		t.Fatalf("по id: %+v", byID)
	}

	exp := time.Now().Add(time.Hour)
	if err := st.SaveRefresh(ctx, "h1", "u1", exp); err != nil {
		t.Fatal(err)
	}
	owner, ok, _ := st.RefreshOwner(ctx, "h1")
	if !ok || owner != "u1" {
		t.Fatalf("refresh owner: %q ok=%v", owner, ok)
	}
	// Истёкший не отдаётся.
	_ = st.SaveRefresh(ctx, "h2", "u1", time.Now().Add(-time.Hour))
	if _, ok, _ := st.RefreshOwner(ctx, "h2"); ok {
		t.Fatal("истёкший refresh принят")
	}
	// Ротация: удаление гасит.
	_ = st.DeleteRefresh(ctx, "h1")
	if _, ok, _ := st.RefreshOwner(ctx, "h1"); ok {
		t.Fatal("удалённый refresh ещё живёт")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestMemStoreUsersAndRefresh -v`
Expected: FAIL — `st.UpsertUser undefined`.

- [ ] **Step 3: Write minimal implementation**

Add `UserID` to `SessionRecord` (in `server/store.go`):

```go
type SessionRecord struct {
	ChatID      string
	CaseID      string
	Seed        int64
	Snapshot    string
	CoreVersion string
	UserID      string // владелец; пусто у легаси-сессий
}
```

Add to the `Store` interface the six methods above. Then in `memStore` add fields + methods:

```go
type memRefresh struct {
	userID string
	exp    time.Time
}

// (в memStore добавить поля)
//   users    map[string]UserRecord // by ID
//   bySub    map[string]string     // google_sub -> ID
//   refresh  map[string]memRefresh // hash -> {userID, exp}
// инициализировать в NewMemStore.

func (s *memStore) UpsertUser(_ context.Context, u UserRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
	if u.GoogleSub != "" {
		s.bySub[u.GoogleSub] = u.ID
	}
	return nil
}

func (s *memStore) UserByGoogleSub(_ context.Context, sub string) (UserRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.bySub[sub]
	if !ok {
		return UserRecord{}, false, nil
	}
	u, ok := s.users[id]
	return u, ok, nil
}

func (s *memStore) UserByID(_ context.Context, id string) (UserRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	return u, ok, nil
}

func (s *memStore) SaveRefresh(_ context.Context, hash, userID string, exp time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh[hash] = memRefresh{userID: userID, exp: exp}
	return nil
}

func (s *memStore) RefreshOwner(_ context.Context, hash string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.refresh[hash]
	if !ok || !r.exp.After(time.Now()) {
		return "", false, nil
	}
	return r.userID, true, nil
}

func (s *memStore) DeleteRefresh(_ context.Context, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.refresh, hash)
	return nil
}
```

Add `import "time"` to `server/store.go`. Init the three maps in `NewMemStore`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/ -run TestMemStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/store.go server/store_test.go
git commit -m "feat(server): users и refresh-токены в memStore, SessionRecord.UserID"
```

---

## Task 4: `auth.Service` — GoogleLogin / Refresh / Me

**Files:**
- Create: `auth/service.go`
- Test: `auth/service_test.go`

**Interfaces:**
- Consumes: `Verifier`, `Tokens`, `Identity` (Tasks 1–2).
- Produces:
  - `type UserStore interface { UpsertUser; UserByGoogleSub; UserByID; SaveRefresh; RefreshOwner; DeleteRefresh }` — те же сигнатуры, что в Task 3, но типы `User`/поля объявлены в пакете `auth` (см. ниже), чтобы `auth` не импортировал `server`.
  - `type User struct { ID, GoogleSub, Email, Name, Picture string }`
  - `type Session struct { AccessToken, RefreshToken string; ExpiresAt time.Time; User User }`
  - `func NewService(v Verifier, t *Tokens, s UserStore, newID func() string) *Service`
  - `func (s *Service) GoogleLogin(ctx, idToken string) (Session, error)`
  - `func (s *Service) Refresh(ctx, refresh string) (Session, error)`
  - `func (s *Service) Me(ctx, userID string) (User, bool, error)`

> Note: `server`'s `memStore` will satisfy `auth.UserStore` structurally IF signatures match. To avoid a type mismatch between `server.UserRecord` and `auth.User`, `UserStore` is defined over `auth.User`. In Task 6, `server` adapts its store to `auth.UserStore` with a thin wrapper (mapping `UserRecord`↔`User`). Keep field names identical.

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"context"
	"testing"
	"time"
)

// memUserStore — хранилище пользователей в памяти для тестов пакета auth.
type memUserStore struct {
	users   map[string]User
	bySub   map[string]string
	refresh map[string]struct {
		uid string
		exp time.Time
	}
}

func newMemUserStore() *memUserStore {
	return &memUserStore{
		users:   map[string]User{},
		bySub:   map[string]string{},
		refresh: map[string]struct {
			uid string
			exp time.Time
		}{},
	}
}
func (m *memUserStore) UpsertUser(_ context.Context, u User) error {
	m.users[u.ID] = u
	if u.GoogleSub != "" {
		m.bySub[u.GoogleSub] = u.ID
	}
	return nil
}
func (m *memUserStore) UserByGoogleSub(_ context.Context, sub string) (User, bool, error) {
	id, ok := m.bySub[sub]
	if !ok {
		return User{}, false, nil
	}
	return m.users[id], true, nil
}
func (m *memUserStore) UserByID(_ context.Context, id string) (User, bool, error) {
	u, ok := m.users[id]
	return u, ok, nil
}
func (m *memUserStore) SaveRefresh(_ context.Context, hash, uid string, exp time.Time) error {
	m.refresh[hash] = struct {
		uid string
		exp time.Time
	}{uid, exp}
	return nil
}
func (m *memUserStore) RefreshOwner(_ context.Context, hash string) (string, bool, error) {
	r, ok := m.refresh[hash]
	if !ok || !r.exp.After(time.Now()) {
		return "", false, nil
	}
	return r.uid, true, nil
}
func (m *memUserStore) DeleteRefresh(_ context.Context, hash string) error {
	delete(m.refresh, hash)
	return nil
}

func svc(t *testing.T, v Verifier) (*Service, *memUserStore) {
	t.Helper()
	store := newMemUserStore()
	seq := 0
	newID := func() string { seq++; return "user-" + string(rune('0'+seq)) }
	return NewService(v, NewTokens("s", nil), store, newID), store
}

func TestGoogleLoginCreatesThenFindsUser(t *testing.T) {
	s, store := svc(t, &FakeVerifier{ID: Identity{Sub: "g1", Email: "a@b.c", Name: "Ann"}})
	first, err := s.GoogleLogin(context.Background(), "id")
	if err != nil || first.AccessToken == "" || first.RefreshToken == "" {
		t.Fatalf("первый вход: %+v err=%v", first, err)
	}
	if first.User.Email != "a@b.c" {
		t.Fatalf("почта: %q", first.User.Email)
	}
	second, _ := s.GoogleLogin(context.Background(), "id")
	if second.User.ID != first.User.ID {
		t.Fatalf("второй вход создал нового пользователя: %q != %q", second.User.ID, first.User.ID)
	}
	if len(store.users) != 1 {
		t.Fatalf("пользователей %d, ждали 1", len(store.users))
	}
}

func TestGoogleLoginRejectsBadToken(t *testing.T) {
	s, _ := svc(t, &FakeVerifier{Err: ErrInvalid})
	if _, err := s.GoogleLogin(context.Background(), "bad"); err == nil {
		t.Fatal("битый ID-token принят")
	}
}

func TestRefreshRotates(t *testing.T) {
	s, _ := svc(t, &FakeVerifier{ID: Identity{Sub: "g1", Email: "a@b.c"}})
	login, _ := s.GoogleLogin(context.Background(), "id")
	next, err := s.Refresh(context.Background(), login.RefreshToken)
	if err != nil || next.AccessToken == "" {
		t.Fatalf("refresh: %+v err=%v", next, err)
	}
	if next.RefreshToken == login.RefreshToken {
		t.Fatal("refresh не ротирован")
	}
	// Старый refresh больше не работает.
	if _, err := s.Refresh(context.Background(), login.RefreshToken); err == nil {
		t.Fatal("старый refresh принят после ротации")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./auth/ -run 'TestGoogleLogin|TestRefresh' -v`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Write minimal implementation**

```go
package auth

import (
	"context"
	"time"
)

type User struct{ ID, GoogleSub, Email, Name, Picture string }

type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	User         User
}

type UserStore interface {
	UpsertUser(ctx context.Context, u User) error
	UserByGoogleSub(ctx context.Context, sub string) (User, bool, error)
	UserByID(ctx context.Context, id string) (User, bool, error)
	SaveRefresh(ctx context.Context, hash, userID string, expiresAt time.Time) error
	RefreshOwner(ctx context.Context, hash string) (string, bool, error)
	DeleteRefresh(ctx context.Context, hash string) error
}

type Service struct {
	v     Verifier
	t     *Tokens
	store UserStore
	newID func() string
}

func NewService(v Verifier, t *Tokens, s UserStore, newID func() string) *Service {
	return &Service{v: v, t: t, store: s, newID: newID}
}

// GoogleLogin проверяет ID-token, находит-или-создаёт пользователя и выдаёт
// сессию (access+refresh). Первый вход = регистрация.
func (s *Service) GoogleLogin(ctx context.Context, idToken string) (Session, error) {
	id, err := s.v.Verify(ctx, idToken)
	if err != nil {
		return Session{}, err
	}
	u, ok, err := s.store.UserByGoogleSub(ctx, id.Sub)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		u = User{ID: s.newID(), GoogleSub: id.Sub, Email: id.Email, Name: id.Name, Picture: id.Picture}
		if err := s.store.UpsertUser(ctx, u); err != nil {
			return Session{}, err
		}
	}
	return s.issue(ctx, u)
}

// Refresh обменивает refresh на новую пару, гася старый (ротация).
func (s *Service) Refresh(ctx context.Context, refresh string) (Session, error) {
	hash := HashRefresh(refresh)
	uid, ok, err := s.store.RefreshOwner(ctx, hash)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		return Session{}, ErrInvalid
	}
	if err := s.store.DeleteRefresh(ctx, hash); err != nil {
		return Session{}, err
	}
	u, ok, err := s.store.UserByID(ctx, uid)
	if err != nil || !ok {
		return Session{}, ErrInvalid
	}
	return s.issue(ctx, u)
}

func (s *Service) Me(ctx context.Context, userID string) (User, bool, error) {
	return s.store.UserByID(ctx, userID)
}

func (s *Service) issue(ctx context.Context, u User) (Session, error) {
	access, exp, err := s.t.IssueAccess(u.ID)
	if err != nil {
		return Session{}, err
	}
	raw, hash, rexp, err := s.t.NewRefresh()
	if err != nil {
		return Session{}, err
	}
	if err := s.store.SaveRefresh(ctx, hash, u.ID, rexp); err != nil {
		return Session{}, err
	}
	return Session{AccessToken: access, RefreshToken: raw, ExpiresAt: exp, User: u}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./auth/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add auth/service.go auth/service_test.go
git commit -m "feat(auth): Service — GoogleLogin/Refresh(ротация)/Me"
```

---

## Task 5: pgStore — users/refresh + миграция 0002

**Files:**
- Create: `server/migrations/0002_auth.sql`
- Modify: `server/pgstore.go`
- Test: `server/pgstore_test.go` (добавить, gated `DATABASE_URL`)

**Interfaces:**
- Consumes: `UserRecord`, `Store` методы (Task 3).
- Produces: pg-реализация шести методов Task 3 + применение `0002` в `migrate`.

- [ ] **Step 1: Write the migration**

`server/migrations/0002_auth.sql`:

```sql
CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,
    google_sub TEXT UNIQUE,
    email      TEXT NOT NULL,
    name       TEXT,
    picture    TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS user_id TEXT REFERENCES users(id);
```

- [ ] **Step 2: Write the failing test (gated)**

Add to `server/pgstore_test.go`:

```go
func TestPgStoreUsersAndRefresh(t *testing.T) {
	st := pgStoreForTest(t) // Skip без DATABASE_URL
	defer st.Close(context.Background())
	ctx := context.Background()

	id := "u-" + randToken()
	sub := "g-" + randToken()
	if err := st.UpsertUser(ctx, UserRecord{ID: id, GoogleSub: sub, Email: "a@b.c", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := st.UserByGoogleSub(ctx, sub)
	if !ok || got.ID != id {
		t.Fatalf("по sub: %+v ok=%v", got, ok)
	}
	// upsert идемпотентен по google_sub
	if err := st.UpsertUser(ctx, UserRecord{ID: id, GoogleSub: sub, Email: "a2@b.c", Name: "Ann2"}); err != nil {
		t.Fatalf("повторный upsert: %v", err)
	}

	h := "h-" + randToken()
	if err := st.SaveRefresh(ctx, h, id, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	owner, ok, _ := st.RefreshOwner(ctx, h)
	if !ok || owner != id {
		t.Fatalf("refresh owner: %q ok=%v", owner, ok)
	}
	_ = st.DeleteRefresh(ctx, h)
	if _, ok, _ := st.RefreshOwner(ctx, h); ok {
		t.Fatal("удалённый refresh жив")
	}
}
```

- [ ] **Step 3: Run to verify it fails (or skips)**

Run: `go test ./server/ -run TestPgStoreUsersAndRefresh -v`
Expected: SKIP без `DATABASE_URL`; с живым Postgres — FAIL (`UpsertUser` undefined on pgStore).

- [ ] **Step 4: Write minimal implementation**

Add `//go:embed migrations/0002_auth.sql` var and run it in `migrate` after 0001:

```go
//go:embed migrations/0002_auth.sql
var migration0002 string

// в migrate(), после Exec(migration0001):
if _, err := s.pool.Exec(ctx, migration0002); err != nil {
	return fmt.Errorf("миграция 0002: %w", err)
}
```

Add pg methods (mirror memStore semantics):

```go
func (s *pgStore) UpsertUser(ctx context.Context, u UserRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, google_sub, email, name, picture)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (google_sub) DO UPDATE SET
			email=EXCLUDED.email, name=EXCLUDED.name, picture=EXCLUDED.picture`,
		u.ID, u.GoogleSub, u.Email, u.Name, u.Picture)
	return err
}

func (s *pgStore) UserByGoogleSub(ctx context.Context, sub string) (UserRecord, bool, error) {
	return s.scanUser(ctx, `SELECT id,google_sub,email,name,picture FROM users WHERE google_sub=$1`, sub)
}
func (s *pgStore) UserByID(ctx context.Context, id string) (UserRecord, bool, error) {
	return s.scanUser(ctx, `SELECT id,google_sub,email,name,picture FROM users WHERE id=$1`, id)
}
func (s *pgStore) scanUser(ctx context.Context, sql string, arg string) (UserRecord, bool, error) {
	var u UserRecord
	var sub, name, pic *string
	err := s.pool.QueryRow(ctx, sql, arg).Scan(&u.ID, &sub, &u.Email, &name, &pic)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	if err != nil {
		return UserRecord{}, false, err
	}
	u.GoogleSub, u.Name, u.Picture = deref(sub), deref(name), deref(pic)
	return u, true, nil
}
func deref(p *string) string { if p == nil { return "" }; return *p }

func (s *pgStore) SaveRefresh(ctx context.Context, hash, userID string, exp time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at) VALUES ($1,$2,$3)`,
		hash, userID, exp)
	return err
}
func (s *pgStore) RefreshOwner(ctx context.Context, hash string) (string, bool, error) {
	var uid string
	err := s.pool.QueryRow(ctx, `
		SELECT user_id FROM refresh_tokens WHERE token_hash=$1 AND expires_at > now()`, hash).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return uid, err == nil, err
}
func (s *pgStore) DeleteRefresh(ctx context.Context, hash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash=$1`, hash)
	return err
}
```

Add `"errors"` and `"time"` imports to `pgstore.go` if missing.

- [ ] **Step 5: Verify + commit**

Run against local throwaway Postgres (as in фаза 6): `DATABASE_URL=... go test ./server/ -run TestPgStore -v` → PASS. Without DB: SKIP.

```bash
git add server/migrations/0002_auth.sql server/pgstore.go server/pgstore_test.go
git commit -m "feat(server): pgStore users/refresh + миграция 0002"
```

---

## Task 6: HTTP-хендлеры auth + адаптер store→auth.UserStore

**Files:**
- Create: `server/authhttp.go`
- Modify: `server/server.go` (маршруты, поле `auth *auth.Service`, флаг `devAuth`)
- Test: `server/authhttp_test.go`

**Interfaces:**
- Consumes: `auth.Service`, `auth.User` (Task 4); `Store` (Task 3).
- Produces:
  - адаптер `type userStoreAdapter struct{ st Store }` реализует `auth.UserStore` (маппинг `UserRecord`↔`auth.User`).
  - `New(mgr, opts...)` принимает `WithAuth(*auth.Service, devAuth bool)`.
  - хендлеры `handleGoogleLogin`, `handleRefresh`, `handleMe`, `handleDevLogin`.

- [ ] **Step 1: Write the failing test**

```go
func testServerWithAuth(t *testing.T) (*Server, *auth.Service) {
	t.Helper()
	mgr := NewManager(casesRoot)
	adapter := userStoreAdapter{st: mgr.store}
	seq := 0
	svc := auth.NewService(&auth.FakeVerifier{ID: auth.Identity{Sub: "g1", Email: "a@b.c", Name: "Ann"}},
		auth.NewTokens("secret", nil), adapter, func() string { seq++; return "u" + string(rune('0'+seq)) })
	srv := New(mgr, WithAuth(svc, true)) // devAuth=true
	return srv, svc
}

func TestGoogleLoginEndpoint(t *testing.T) {
	srv, _ := testServerWithAuth(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/auth/google",
		strings.NewReader(`{"id_token":"x"}`)))
	if rec.Code != 200 {
		t.Fatalf("код %d тело %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		User    struct{ Email string } `json:"user"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Access == "" || out.Refresh == "" || out.User.Email != "a@b.c" {
		t.Fatalf("ответ: %+v", out)
	}
}

func TestDevLoginBlockedWithoutFlag(t *testing.T) {
	mgr := NewManager(casesRoot)
	svc := auth.NewService(&auth.FakeVerifier{}, auth.NewTokens("s", nil), userStoreAdapter{st: mgr.store}, func() string { return "u" })
	srv := New(mgr, WithAuth(svc, false)) // devAuth=false
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/auth/dev", strings.NewReader(`{"email":"a@b.c"}`)))
	if rec.Code != 404 {
		t.Fatalf("dev-login без флага: код %d, ждали 404", rec.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./server/ -run 'TestGoogleLoginEndpoint|TestDevLoginBlocked' -v`
Expected: FAIL — `undefined: WithAuth`.

- [ ] **Step 3: Write minimal implementation**

`server/authhttp.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kliuchnikovv/dnd/auth"
)

// userStoreAdapter приводит серверный Store к auth.UserStore (auth не знает о
// server). Поля UserRecord и auth.User совпадают по именам.
type userStoreAdapter struct{ st Store }

func (a userStoreAdapter) UpsertUser(ctx context.Context, u auth.User) error {
	return a.st.UpsertUser(ctx, UserRecord(u))
}
func (a userStoreAdapter) UserByGoogleSub(ctx context.Context, sub string) (auth.User, bool, error) {
	r, ok, err := a.st.UserByGoogleSub(ctx, sub)
	return auth.User(r), ok, err
}
func (a userStoreAdapter) UserByID(ctx context.Context, id string) (auth.User, bool, error) {
	r, ok, err := a.st.UserByID(ctx, id)
	return auth.User(r), ok, err
}
func (a userStoreAdapter) SaveRefresh(ctx context.Context, hash, userID string, exp time.Time) error {
	return a.st.SaveRefresh(ctx, hash, userID, exp)
}
func (a userStoreAdapter) RefreshOwner(ctx context.Context, hash string) (string, bool, error) {
	return a.st.RefreshOwner(ctx, hash)
}
func (a userStoreAdapter) DeleteRefresh(ctx context.Context, hash string) error {
	return a.st.DeleteRefresh(ctx, hash)
}

func (s *Server) handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IDToken == "" {
		writeError(w, http.StatusBadRequest, "нужен id_token")
		return
	}
	sess, err := s.auth.GoogleLogin(r.Context(), req.IDToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "вход отклонён")
		return
	}
	writeSession(w, sess)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "нужен refresh_token")
		return
	}
	sess, err := s.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "refresh отклонён")
		return
	}
	writeSession(w, sess)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	u, ok, err := s.auth.Me(r.Context(), uid)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "нет пользователя")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// handleDevLogin — вход без Google для локали/тестов. Работает только при devAuth.
func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !s.devAuth {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "нужен email")
		return
	}
	// Тот же путь, что GoogleLogin, но личность берётся из запроса. Реализуется
	// через Service с FakeVerifier, настроенным на этот email? Нет — используем
	// прямой метод Service.DevLogin (добавить в Task 4? нет: делаем здесь через
	// UpsertUser+issue недоступно). Проще: Service принимает Identity напрямую.
	sess, err := s.auth.LoginAs(r.Context(), auth.Identity{Sub: "dev:" + req.Email, Email: req.Email, Name: req.Email})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSession(w, sess)
}

func writeSession(w http.ResponseWriter, s auth.Session) {
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  s.AccessToken,
		"refresh_token": s.RefreshToken,
		"expires_at":    s.ExpiresAt.Unix(),
		"user":          s.User,
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}
```

Add to `auth/service.go` a `LoginAs(ctx, Identity)` used by dev-login (find-or-create + issue, no verifier):

```go
// LoginAs входит под заданной личностью без проверки Google — для DEV_AUTH.
func (s *Service) LoginAs(ctx context.Context, id Identity) (Session, error) {
	u, ok, err := s.store.UserByGoogleSub(ctx, id.Sub)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		u = User{ID: s.newID(), GoogleSub: id.Sub, Email: id.Email, Name: id.Name, Picture: id.Picture}
		if err := s.store.UpsertUser(ctx, u); err != nil {
			return Session{}, err
		}
	}
	return s.issue(ctx, u)
}
```

(Refactor `GoogleLogin` to call `LoginAs` after `Verify`.) Add a test for `LoginAs` in `auth/service_test.go`.

In `server/server.go`: add fields `auth *auth.Service`, `devAuth bool`; option:

```go
type Option func(*Server)

func WithAuth(a *auth.Service, devAuth bool) Option {
	return func(s *Server) { s.auth = a; s.devAuth = devAuth }
}
```

Change `New(mgr *Manager, opts ...Option) *Server` to apply opts, and register routes (only if `s.auth != nil`):

```go
s.mux.HandleFunc("POST /auth/google", s.handleGoogleLogin)
s.mux.HandleFunc("POST /auth/refresh", s.handleRefresh)
s.mux.HandleFunc("GET /me", s.requireAuth(s.handleMe))
s.mux.HandleFunc("POST /auth/dev", s.handleDevLogin)
```

Add `"time"` import to `authhttp.go`. `requireAuth` and `userIDFrom` come from Task 7 — for this task, stub `requireAuth` as identity wrapper returning 401 when no token, and `userIDFrom`; OR sequence Task 7 before wiring `/me`. (Executor: implement Task 7's middleware here if needed; they're one review unit if you prefer — but keep commits separate.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/ ./auth/ -run 'TestGoogleLoginEndpoint|TestDevLoginBlocked|TestLoginAs' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/authhttp.go server/server.go auth/service.go auth/service_test.go server/authhttp_test.go
git commit -m "feat(server): хендлеры /auth/google,/auth/refresh,/me,/auth/dev + адаптер store"
```

---

## Task 7: Гейт REST/WS + владение chat_id

**Files:**
- Modify: `server/authhttp.go` (middleware, ctx-ключ), `server/server.go` (обернуть `/sessions`), `server/ws.go` (userID из токена, проверка владения), `server/manager.go` (`Create` с userID; `Get`→владелец)
- Test: `server/authgate_test.go`

**Interfaces:**
- Consumes: `Server.auth` (Task 6), `SessionRecord.UserID` (Task 3).
- Produces:
  - `func userIDFrom(ctx context.Context) string`
  - `func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc`
  - `Manager.Create(caseName string, seed int64, userID string) (string, error)` (сигнатура меняется)
  - `sessionRuntime.userID string`; проверка в `handleWS`.

- [ ] **Step 1: Write the failing test**

```go
func TestSessionsRequiresAuth(t *testing.T) {
	srv, _ := testServerWithAuth(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"case":"harbour"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена ждали 401, получили %d", rec.Code)
	}
}

func TestWSRejectsForeignChatID(t *testing.T) {
	srv, svc := testServerWithAuth(t)
	// пользователь A создаёт сессию
	a, _ := svc.LoginAs(context.Background(), auth.Identity{Sub: "gA", Email: "a@x"})
	idA := createSessionAs(t, srv, a.AccessToken, "harbour")
	// пользователь B пытается подключиться к сессии A
	b, _ := svc.LoginAs(context.Background(), auth.Identity{Sub: "gB", Email: "b@x"})

	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	url := "ws" + strings.TrimPrefix(hs.URL, "http") + "/chat/ws?token=" + b.AccessToken + "&chat_id=" + idA
	_, resp, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatal("чужой chat_id принят")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("ждали 403, получили %v", resp)
	}
}
```

(Helper `createSessionAs` does `POST /sessions` with `Authorization: Bearer`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./server/ -run 'TestSessionsRequiresAuth|TestWSRejectsForeignChatID' -v`
Expected: FAIL (сейчас `/sessions` без гейта, WS не проверяет владельца).

- [ ] **Step 3: Write minimal implementation**

Middleware + ctx key in `authhttp.go`:

```go
type ctxKey struct{}

func userIDFrom(ctx context.Context) string { s, _ := ctx.Value(ctxKey{}).(string); return s }

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, err := s.auth.Verify(bearer(r)) // см. ниже
		if err != nil {
			writeError(w, http.StatusUnauthorized, "нужна авторизация")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, uid)))
	}
}
```

Add `Service.Verify(access string) (string, error)` delegating to `Tokens.VerifyAccess` (add to `auth/service.go`; expose `t`):

```go
func (s *Service) Verify(access string) (string, error) { return s.t.VerifyAccess(access) }
```

Wrap `/sessions` with `requireAuth`; in `handleCreateSession`, pass `userIDFrom(r.Context())` to `mgr.Create`. Change `Manager.Create` signature to accept `userID`, store it in `SessionRecord.UserID` and `sessionRuntime.userID`. Update the reconstruct path to carry `UserID`.

In `handleWS`: replace anon-token check with real verify + ownership:

```go
uid, err := s.auth.Verify(q.Get("token"))
if err != nil {
	http.Error(w, "нужна авторизация", http.StatusUnauthorized)
	return
}
rt, ok := s.mgr.Get(chatID)
if !ok {
	http.Error(w, "нет такой сессии", http.StatusNotFound)
	return
}
if rt.userID != uid {
	http.Error(w, "чужая сессия", http.StatusForbidden)
	return
}
```

Update every `mgr.Create(case, seed)` call in existing tests to `mgr.Create(case, seed, "test-user")` (Task 8 handles the sweep, but this task's own new tests use the authed helpers).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/ -run 'TestSessionsRequiresAuth|TestWSRejectsForeignChatID' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/authhttp.go server/server.go server/ws.go server/manager.go server/authgate_test.go auth/service.go
git commit -m "feat(server): гейт REST/WS и владение chat_id"
```

---

## Task 8: Строгий старт cmd/server + свип существующих тестов + allowlist

**Files:**
- Modify: `cmd/server/main.go`, `cmd/server/narrator.go` (env), `e2e/architecture_test.go`, все существующие `server/*_test.go` вызовы `Create`/`New`/WS без токена.

**Interfaces:**
- Consumes: всё выше.
- Produces: рабочий бинарь со строгим гейтом; зелёная сюита.

- [ ] **Step 1: Update existing server tests to authenticate**

Every existing test that calls `m.Create("harbour", 1)` becomes `m.Create("harbour", 1, "test-user")`. Every WS dial helper (`wsDial`) and `New(mgr)` gets an auth-enabled server + a minted token. Introduce a shared test helper:

```go
// authedTestServer — сервер с DEV_AUTH и один заранее вошедший пользователь.
func authedTestServer(t *testing.T, mgr *Manager) (*Server, string) {
	t.Helper()
	svc := auth.NewService(&auth.FakeVerifier{ID: auth.Identity{Sub: "g-test", Email: "t@x"}},
		auth.NewTokens("test-secret", nil), userStoreAdapter{st: mgr.store}, func() string { return "test-user" })
	sess, _ := svc.LoginAs(context.Background(), auth.Identity{Sub: "g-test", Email: "t@x"})
	return New(mgr, WithAuth(svc, true)), sess.AccessToken
}
```

Update `wsDial` to take a token and put it in `?token=`. Update `New(NewManager(...))` sites to `authedTestServer`. Sessions in these tests are created under `"test-user"`, matching the token's user.

- [ ] **Step 2: Run the whole server suite**

Run: `go test ./server/ -v`
Expected: PASS (all prior tests now authenticate).

- [ ] **Step 3: Strict startup + wiring in cmd/server**

In `cmd/server/main.go` `buildManager` (or a new `buildAuth`): read `JWT_SECRET` (fatal if empty), `GOOGLE_CLIENT_ID`, `DEV_AUTH`. Build verifier: if `DEV_AUTH=1` and no client id → allow `FakeVerifier`-free path via dev endpoint; else `auth.NewGoogleVerifier(ctx, clientID)`. Build `auth.Service` over the chosen store adapter with `uuid.NewString` as `newID`. Pass `server.New(mgr, server.WithAuth(svc, devAuth))`.

```go
secret := os.Getenv("JWT_SECRET")
if secret == "" {
	log.Fatalf("JWT_SECRET обязателен: сервер без него открыт")
}
```

- [ ] **Step 4: Allowlist + arch guard**

Add to `e2e/architecture_test.go` allowlist: `google.golang.org/api/idtoken`, `github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`. Optionally assert `auth` is not imported by pureLayers.

Run: `go test ./e2e/ -run TestDependenciesAreAllowlisted -v && go test ./... `
Expected: PASS.

- [ ] **Step 5: Verify build + live smoke + commit**

```bash
go build -o /tmp/dndserver ./cmd/server
# строгий старт: без JWT_SECRET — фатал
/tmp/dndserver ; echo "exit=$?"   # ждём фатал про JWT_SECRET
# с секретом и DEV_AUTH — стартует; POST /auth/dev выдаёт токен; POST /sessions с Bearer работает
```

```bash
git add cmd/server/ e2e/architecture_test.go server/
git commit -m "feat(server): строгий старт, гейт по умолчанию, тесты под токеном"
```

---

## Self-Review

**Spec coverage:** §A.1 API → Tasks 6–7; §A.2 данные → Tasks 3,5; §A.3 пакет auth → Tasks 1,2,4; §A.4 гейт → Task 7; §A.5 конфиг/deps → Tasks 1,2,8; §A.6 тесты → каждая задача; §A.7 фазы → Tasks 1–8. Инвариант «auth над доменом» → Task 8 arch guard. Покрыто.

**Placeholder scan:** в Task 6 dev-login описан через `LoginAs` (реальный метод, добавлен в Task 6 step 3), не заглушка. Прочие шаги несут реальный код/тесты.

**Type consistency:** `UserRecord` (server) ↔ `auth.User` — одинаковые поля, мост `userStoreAdapter` конвертирует прямым `UserRecord(u)`/`auth.User(r)`. `Store` методы users/refresh (Task 3) = `auth.UserStore` сигнатуры (Task 4) по типам после адаптера. `Manager.Create` меняет арность в Task 7 — Task 8 правит все вызовы. `Service.Verify`/`LoginAs` добавлены явно.

## Границы

Email/пароль, `NetSource`, мультиаккаунт, rate-limit, ротация `JWT_SECRET` — не здесь (см. спеку §Границы).
