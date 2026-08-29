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

func TestParseClientIDs(t *testing.T) {
	cases := map[string][]string{
		"":                       nil,
		"   ":                    nil,
		"a":                      {"a"},
		"a,b":                    {"a", "b"},
		" a , b ,":               {"a", "b"},
		"ios.x , web.x ,, ,and.x": {"ios.x", "web.x", "and.x"},
	}
	for in, want := range cases {
		got := ParseClientIDs(in)
		if len(got) != len(want) {
			t.Fatalf("ParseClientIDs(%q) = %v, ждали %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("ParseClientIDs(%q)[%d] = %q, ждали %q", in, i, got[i], want[i])
			}
		}
	}
}

func TestNewGoogleVerifierRejectsEmptyList(t *testing.T) {
	if _, err := NewGoogleVerifier(context.Background(), nil); err == nil {
		t.Fatal("пустой список client id должен быть ошибкой")
	}
}
