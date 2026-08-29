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
