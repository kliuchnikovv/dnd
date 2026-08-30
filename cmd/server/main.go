// Команда server — тонкий транспорт над доменом: HTTP для health и старта
// сессии, WebSocket для реального времени (следующие фазы). Игровой логики
// здесь нет — состояние живёт в ядре.
//
// Конфигурация из окружения (Railway): PORT — порт (по умолчанию 8080),
// CASES_DIR — корень дел (по умолчанию "cases").
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kliuchnikovv/dnd/server"
)

func main() {
	addr := ":" + env("PORT", "8080")
	casesDir := env("CASES_DIR", "cases")

	// Журнал: Postgres, если задан DATABASE_URL (прод/Railway), иначе память
	// (локальный прогон без БД). Реплей и восстановление сессии одинаковы для
	// обоих — разнится только долговечность.
	mgr, closeStore := buildManager(casesDir)
	defer closeStore()

	authCtx, cancelAuth := context.WithTimeout(context.Background(), 30*time.Second)
	svc, devAuth := buildAuth(authCtx, mgr)
	cancelAuth()

	srv := server.New(mgr, server.WithAuth(svc, devAuth))
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Останов по сигналу: перестаём принимать новое и даём текущему
	// дозавершиться. В следующих фазах сюда добавится закрытие сокетов и
	// дослив буферов прозы.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		log.Printf("server слушает %s (дела: %s)", addr, casesDir)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("получен сигнал останова, завершаемся")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown: %v", err)
	}
	log.Printf("остановлен чисто")
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// buildManager выбирает журнал по окружению. С DATABASE_URL — Postgres: копит
// миграции, прогревает кэш живыми сессиями (клиент после рестарта не ждёт
// ленивого восстановления). Без него — журнал в памяти. Возвращает менеджер и
// функцию закрытия ресурсов для graceful shutdown.
func buildManager(casesDir string) (*server.Manager, func()) {
	narrator, parser, err := buildNarrator()
	if err != nil {
		log.Fatalf("проза: %v", err)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Printf("журнал: в памяти (DATABASE_URL не задан)")
		return server.NewManager(casesDir).WithNarrator(narrator).WithInterpreter(parser), func() {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := server.NewPgStore(ctx, dsn)
	if err != nil {
		log.Fatalf("журнал Postgres: %v", err)
	}
	mgr := server.NewManagerWithStore(casesDir, st).WithNarrator(narrator).WithInterpreter(parser)
	if err := mgr.WarmCache(ctx); err != nil {
		log.Fatalf("прогрев кэша сессий: %v", err)
	}
	log.Printf("журнал: Postgres")
	return mgr, func() { _ = st.Close(context.Background()) }
}
