// Строгий гейт входа: сервер не стартует без JWT_SECRET, а без
// GOOGLE_CLIENT_ID работает только при явном DEV_AUTH=1 (локаль/CI).
package main

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/google/uuid"

	"github.com/kliuchnikovv/dnd/auth"
	"github.com/kliuchnikovv/dnd/server"
)

// buildAuth собирает auth.Service поверх менеджера и решает, разрешён ли
// DEV_AUTH. Фатально останавливает процесс, если конфигурация открывает
// сервер всем: без JWT_SECRET и без GOOGLE_CLIENT_ID+DEV_AUTH.
func buildAuth(ctx context.Context, mgr *server.Manager) (svc *auth.Service, devAuth bool) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatalf("JWT_SECRET обязателен: сервер без него открыт для всех")
	}

	devAuth = os.Getenv("DEV_AUTH") == "1"
	// GOOGLE_CLIENT_ID — один или несколько client id через запятую: на разных
	// платформах Google кладёт в aud разный client (iOS/web/Android), и токен
	// проходит по любому из заявленных.
	clientIDs := auth.ParseClientIDs(os.Getenv("GOOGLE_CLIENT_ID"))

	var verifier auth.Verifier
	switch {
	case len(clientIDs) > 0:
		v, err := auth.NewGoogleVerifier(ctx, clientIDs)
		if err != nil {
			log.Fatalf("Google verifier: %v", err)
		}
		verifier = v
		log.Printf("авторизация: Google (%d client id)", len(clientIDs))
	case devAuth:
		verifier = &auth.FakeVerifier{Err: errors.New("GoogleLogin недоступен: задайте GOOGLE_CLIENT_ID")}
		log.Printf("авторизация: только DEV_AUTH (GOOGLE_CLIENT_ID не задан)")
	default:
		log.Fatalf("GOOGLE_CLIENT_ID обязателен (или DEV_AUTH=1 для локального входа)")
	}

	svc = mgr.AuthService(verifier, secret, nil, uuid.NewString)
	return svc, devAuth
}
