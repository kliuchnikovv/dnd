package auth

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/idtoken"
)

// Verifier проверяет Google ID-token и извлекает личность. Интерфейс — чтобы
// тесты и DEV_AUTH не ходили в сеть.
type Verifier interface {
	Verify(ctx context.Context, idToken string) (Identity, error)
}

type googleVerifier struct {
	validator *idtoken.Validator
	// clientIDs — допустимые audience. Их несколько, потому что на разных
	// платформах Google кладёт в aud РАЗНЫЙ client: нативный iOS-вход даёт
	// aud=iOS-client, web/бэкенд-паттерн — web-client, Android — свой. Токен
	// проходит, если его aud совпал с любым из заявленных.
	clientIDs []string
}

// ParseClientIDs разбирает список client-id из одной строки env (через запятую):
// пробелы срезаются, пустые отбрасываются. Пустой вход — пустой список.
func ParseClientIDs(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if id := strings.TrimSpace(part); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// NewGoogleVerifier — прод-проверка против accounts.google.com. Токен принят,
// если его audience совпал с одним из clientIDs. Пустой список — ошибка: без
// заявленной аудитории проверять нечего (любой валидный Google-токен прошёл бы).
func NewGoogleVerifier(ctx context.Context, clientIDs []string) (Verifier, error) {
	if len(clientIDs) == 0 {
		return nil, fmt.Errorf("auth: нужен хотя бы один Google client id")
	}
	v, err := idtoken.NewValidator(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: idtoken validator: %w", err)
	}
	return &googleVerifier{validator: v, clientIDs: clientIDs}, nil
}

func (g *googleVerifier) Verify(ctx context.Context, idToken string) (Identity, error) {
	// Пробуем каждый заявленный audience: Validate проверяет подпись/issuer/
	// срок И совпадение aud с переданным id. Валидатор кэширует ключи Google,
	// поэтому повтор для двух-трёх client id стоит дёшево (без новой сети).
	var lastErr error
	for _, id := range g.clientIDs {
		p, err := g.validator.Validate(ctx, idToken, id)
		if err != nil {
			lastErr = err
			continue
		}
		return Identity{
			Sub:     p.Subject,
			Email:   str(p.Claims["email"]),
			Name:    str(p.Claims["name"]),
			Picture: str(p.Claims["picture"]),
		}, nil
	}
	return Identity{}, fmt.Errorf("auth: id-token не прошёл ни по одному audience: %w", lastErr)
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
