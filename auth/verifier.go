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
