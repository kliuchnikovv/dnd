package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/kliuchnikovv/dnd/llm"
)

// buildGateway собирает llm.Gateway из окружения по ДОМЕННОЙ конвенции (как
// cmd/server, НЕ прото-схема per-role env): один MODEL маршрутизируется роутером
// на все нужные роли. Ключ провайдер читает из окружения сам
// (OPENROUTER_API_KEY / ANTHROPIC_API_KEY). Возвращает (nil, nil), если MODEL не
// задан, — тогда CLI играет офлайн. Это НЕ turn-логика (её единый код в
// vignette.Narrate), а конфиг-обвязка: cmd/server держит свою в package main,
// импортировать нечего — поэтому тонкая копия здесь, а не форк механики.
func buildGateway() (*llm.Gateway, error) {
	model := os.Getenv("MODEL")
	if model == "" {
		return nil, nil
	}
	provider := env("PROVIDER", "openrouter")
	target, err := resolveProvider(provider, model)
	if err != nil {
		return nil, err
	}
	if err := ensurePrice(target.Model, envFloat("PRICE_IN", 0), envFloat("PRICE_OUT", 0)); err != nil {
		return nil, err
	}

	var cheap llm.Target
	if mc := os.Getenv("MODEL_CHEAP"); mc != "" {
		cheap = llm.Target{Provider: target.Provider, Model: mc}
		if err := ensurePrice(mc, envFloat("PRICE_IN_CHEAP", 0), envFloat("PRICE_OUT_CHEAP", 0)); err != nil {
			return nil, err
		}
	}

	capDay := envFloat("CAP_DAY", 1.0)
	router := llm.NewRouter().
		Route(llm.RoleNarrator, target).     // проза Мастера
		Route(llm.RoleCanonGuard, target).   // на будущее (семантический страж — отложен)
		Route(llm.RoleIntentParser, target). // судья виньетки (LLMJudge)
		Route(llm.RoleWorldsmith, target)    // генератор сцен (scenegen)
	if cheap.Provider != nil && cheap.Model != "" {
		router.RouteCheap(llm.RoleNarrator, cheap)
	}
	gw := llm.NewGateway(router, llm.NewLedger(llm.Caps{
		GlobalDailyMicro:   int64(capDay * 1_000_000),
		ForecastDailyMicro: int64(capDay * 1_000_000 / 4),
		PerTurnCalls:       int(envFloat("CAP_TURN", 8)),
	}))
	return gw, nil
}

// resolveProvider — тот же выбор, что в cmd/server/cmd/dnd. Ключ провайдер читает
// из окружения сам.
func resolveProvider(name, model string) (llm.Target, error) {
	switch name {
	case "openrouter":
		return llm.Target{Provider: llm.NewOpenRouter(llm.ORWithTitle("dnd")), Model: model}, nil
	case "anthropic":
		return llm.Target{Provider: llm.NewAnthropic(), Model: model}, nil
	default:
		return llm.Target{}, fmt.Errorf("неизвестный PROVIDER %q: ожидается openrouter или anthropic", name)
	}
}

// ensurePrice регистрирует цену модели, если её нет в таблице (иначе расход не в счёт).
func ensurePrice(model string, in, out float64) error {
	if in > 0 && out > 0 {
		llm.SetPrice(model, llm.Price{
			InputMicroPerMTok:  int64(in * 1_000_000),
			OutputMicroPerMTok: int64(out * 1_000_000),
		})
		return nil
	}
	if llm.HasPrice(model) {
		return nil
	}
	return fmt.Errorf("цена модели %q неизвестна: задайте PRICE_IN и PRICE_OUT "+
		"($/млн токенов), цены на https://openrouter.ai/models", model)
}

// hasAPIKey — есть ли ключ провайдера в окружении (гейт «живого» режима: MODEL
// без ключа — всё равно офлайн, чтобы fake-прогон не маскировался под живой).
func hasAPIKey(provider string) bool {
	switch provider {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY") != ""
	default:
		return os.Getenv("OPENROUTER_API_KEY") != ""
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
