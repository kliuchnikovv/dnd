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
func (m *memUserStore) ClaimRefresh(_ context.Context, hash string) (string, bool, error) {
	r, ok := m.refresh[hash]
	if !ok || !r.exp.After(time.Now()) {
		return "", false, nil
	}
	delete(m.refresh, hash)
	return r.uid, true, nil
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

// TestClaimRefreshSingleUse проверяет атомарность заявки на refresh: первый
// ClaimRefresh забирает владельца и гасит хэш, второй — уже не находит его.
// Это и есть механизм, закрывающий окно гонки параллельного refresh одним и
// тем же токеном (иначе оба запроса успели бы пройти RefreshOwner до того,
// как второй из них выполнит DeleteRefresh).
func TestClaimRefreshSingleUse(t *testing.T) {
	_, store := svc(t, &FakeVerifier{})
	ctx := context.Background()
	hash := "claim-hash"
	if err := store.SaveRefresh(ctx, hash, "user-x", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	uid, ok, err := store.ClaimRefresh(ctx, hash)
	if err != nil || !ok || uid != "user-x" {
		t.Fatalf("первый ClaimRefresh: uid=%q ok=%v err=%v", uid, ok, err)
	}
	uid, ok, err = store.ClaimRefresh(ctx, hash)
	if err != nil || ok || uid != "" {
		t.Fatalf("второй ClaimRefresh должен провалиться: uid=%q ok=%v err=%v", uid, ok, err)
	}
}

func TestServiceVerify(t *testing.T) {
	s, _ := svc(t, &FakeVerifier{ID: Identity{Sub: "g1", Email: "a@b.c"}})
	login, err := s.GoogleLogin(context.Background(), "id")
	if err != nil {
		t.Fatalf("вход: %v", err)
	}
	uid, err := s.Verify(login.AccessToken)
	if err != nil || uid != login.User.ID {
		t.Fatalf("Verify: uid=%q err=%v, ждали %q", uid, err, login.User.ID)
	}
	if _, err := s.Verify("garbage"); err == nil {
		t.Fatal("битый access-токен принят")
	}
}

func TestLoginAs(t *testing.T) {
	s, store := svc(t, &FakeVerifier{})
	id := Identity{Sub: "g2", Email: "b@c.d", Name: "Bob", Picture: "pic"}
	first, err := s.LoginAs(context.Background(), id)
	if err != nil || first.AccessToken == "" || first.RefreshToken == "" {
		t.Fatalf("первый LoginAs: %+v err=%v", first, err)
	}
	if first.User.Email != "b@c.d" {
		t.Fatalf("почта: %q", first.User.Email)
	}
	// Второй LoginAs с тем же Identity находит пользователя
	second, _ := s.LoginAs(context.Background(), id)
	if second.User.ID != first.User.ID {
		t.Fatalf("второй LoginAs создал нового пользователя: %q != %q", second.User.ID, first.User.ID)
	}
	if len(store.users) != 1 {
		t.Fatalf("пользователей %d, ждали 1", len(store.users))
	}
}
