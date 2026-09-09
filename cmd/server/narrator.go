package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/kliuchnikovv/dnd/guard"
	"github.com/kliuchnikovv/dnd/intent"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/scenegen"
	"github.com/kliuchnikovv/dnd/server"
	"github.com/kliuchnikovv/dnd/vignette"
)

// buildNarrator собирает Мастера для стрима прозы из окружения. Прозы нет без
// MODEL: сервер тогда отдаёт только механику (session_state). Конфигурация
// повторяет cmd/dnd, но флаги стали переменными окружения (Railway):
//
//	MODEL           основная модель (пусто -> без прозы)
//	PROVIDER        openrouter | anthropic (по умолчанию openrouter)
//	MODEL_CHEAP     модель дешёвого тира (необязательно)
//	CAP_DAY         суточный потолок расхода, $ (по умолчанию 1.0)
//	CAP_TURN        потолок вызовов на ход (по умолчанию 8)
//	PRICE_IN/OUT    цена модели, $/млн токенов, если её нет в таблице
//	PRICE_IN_CHEAP/OUT_CHEAP  то же для дешёвой модели
//	GUARD_LINES     проверять прозу гвардом (по умолчанию true)
//	OPENROUTER_API_KEY / ANTHROPIC_API_KEY  читают провайдеры сами
func buildNarrator() (*master.Master, *intent.Parser, vignette.Judge, server.SceneGenerator, error) {
	model := os.Getenv("MODEL")
	if model == "" {
		log.Printf("проза: выключена (MODEL не задан) — сервер отдаёт только механику")
		return nil, nil, nil, nil, nil
	}
	provider := env("PROVIDER", "openrouter")
	target, err := resolveProvider(provider, model)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := ensurePrice(target.Model, envFloat("PRICE_IN", 0), envFloat("PRICE_OUT", 0)); err != nil {
		return nil, nil, nil, nil, err
	}

	var cheap llm.Target
	if mc := os.Getenv("MODEL_CHEAP"); mc != "" {
		cheap = llm.Target{Provider: target.Provider, Model: mc}
		if err := ensurePrice(mc, envFloat("PRICE_IN_CHEAP", 0), envFloat("PRICE_OUT_CHEAP", 0)); err != nil {
			return nil, nil, nil, nil, err
		}
	}

	capDay := envFloat("CAP_DAY", 1.0)
	// Разбор свободного ввода — тот же таргет, что у прозы. Чат-парсер (один
	// вызов: разбор + реплика) ходит под RoleChatMaster; RoleIntentParser
	// зароучен на будущее (нечат-режим), лишним не бывает.
	router := llm.NewRouter().
		Route(llm.RoleNarrator, target).
		Route(llm.RoleCanonGuard, target).
		Route(llm.RoleChatMaster, target).
		Route(llm.RoleIntentParser, target).
		Route(llm.RoleWorldsmith, target) // генератор сцен виньетки
	if cheap.Provider != nil && cheap.Model != "" {
		router.RouteCheap(llm.RoleNarrator, cheap)
	}
	gw := llm.NewGateway(router, llm.NewLedger(llm.Caps{
		GlobalDailyMicro:   int64(capDay * 1_000_000),
		ForecastDailyMicro: int64(capDay * 1_000_000 / 4),
		PerTurnCalls:       int(envFloat("CAP_TURN", 8)),
	}))

	ms := master.New(gw)
	if envBool("GUARD_LINES", true) {
		ms = ms.WithGuard(guard.New(gw))
	}
	// Чат-парсер: один вызов даёт и разбор текста, и реплику (для свободного ввода).
	parser := intent.NewChatParser(gw)
	// Судья виньетки на том же шлюзе: разбирает свободный ввод в Ruling холодно и
	// truth-blind (RoleIntentParser), сам откатывается на keyword при сбое.
	vjudge := vignette.NewLLMJudge(gw)

	// Генератор сцен виньетки: тот же шлюз, что и у прозы (RoleWorldsmith
	// заведён выше). Продуктовый путь «тема → SceneSpec → CreateVignette»
	// (см. server.CreateVignetteFromTheme и POST /vignette/from-theme):
	// генератор — core-способность, а не приватная деталь CLI cmd/vignette.
	sceneGen := server.SceneGenerator(func(ctx context.Context, theme, mode string) (*scenegen.SceneSpec, error) {
		spec, _, err := scenegen.Generate(ctx, gw, theme, mode)
		return spec, err
	})

	log.Printf("проза: включена (provider=%s model=%s guard=%v); свободный ввод: включён; судья виньетки: LLM; scenegen: включён",
		provider, model, envBool("GUARD_LINES", true))
	return ms, parser, vjudge, sceneGen, nil
}

// resolveProvider — тот же выбор, что в cmd/dnd. Ключ провайдер читает из
// окружения сам.
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

// ensurePrice регистрирует цену модели, если её нет в таблице. Молча тратить по
// неизвестной цене нельзя — расход тогда не в счёт (см. Gateway).
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

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
