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
