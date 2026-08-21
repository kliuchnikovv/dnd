// Команда dnd — консольный прототип детективной игры.
//
// По умолчанию ни одного вызова модели: ввод структурированный, прогон
// воспроизводится парой (seed, ввод). Флаг -nl включает перевод свободного
// текста — тогда нераспознанный ввод уходит в модель, и воспроизводимость
// сохраняется только для структурированных команд.
//
// Ключ в коде не хранится. OpenRouter читает OPENROUTER_API_KEY, Anthropic —
// ANTHROPIC_API_KEY либо профиль, оставленный ant auth login.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/kliuchnikovv/dnd/actor"
	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/intent"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func main() {
	casePath := flag.String("case", "cases/harbour/case.json", "путь к файлу дела")
	seed := flag.Int64("seed", 1, "seed RNG: прогон воспроизводится парой (seed, ввод)")
	script := flag.String("script", "", "файл команд вместо интерактивного ввода")
	nl := flag.Bool("nl", false, "переводить свободный текст в действия через модель")
	provider := flag.String("provider", "openrouter", "поставщик модели: openrouter | anthropic")
	model := flag.String("model", "", "идентификатор модели; для openrouter обязателен")
	modelCheap := flag.String("model-cheap", "",
		"модель для ходов, не меняющих мир: приветствие, прощание, ремонт реплики, запрос к Мастеру")
	priceInCheap := flag.Float64("price-in-cheap", 0, "цена ввода дешёвой модели, $ за миллион токенов")
	priceOutCheap := flag.Float64("price-out-cheap", 0, "цена вывода дешёвой модели, $ за миллион токенов")
	capDay := flag.Float64("cap-day", 1.0, "потолок расхода в долларах за сутки")
	priceIn := flag.Float64("price-in", 0, "цена ввода, $ за миллион токенов; нужна для моделей вне таблицы")
	priceOut := flag.Float64("price-out", 0, "цена вывода, $ за миллион токенов; нужна для моделей вне таблицы")
	guardLines := flag.Bool("guard-lines", true,
		"проверять реплики NPC на утечку дела вторым вызовом")
	debugLLM := flag.Bool("debug-llm", false, "печатать обмен с моделью целиком")
	// Ход при полном протоколе: разбор ввода, актёр, Мастер за данными, актёр
	// договаривает, проверка реплики, ремонт при утечке, повторная проверка,
	// проза Мастера. Потолок ниже означал бы, что часть надстроек молча
	// отваливается на ErrTurnCalls — и это выглядело бы как плохая модель, а
	// не как исчерпанный лимит.
	capTurn := flag.Int("cap-turn", 8,
		"потолок вызовов модели на один ход: разбор, актёр×2, Мастер, проверка, ремонт, нарратив")
	envFile := flag.String("env", ".env", "файл с переменными окружения; уже заданное окружение приоритетнее")
	flag.Parse()
	loadDotenv(*envFile)

	cfg, err := cases.Load(*casePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(*seed).Stream("resolve")

	in := os.Stdin
	if *script != "" {
		f, err := os.Open(*script)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}

	game := core.NewGame(*cfg)
	session := cli.NewSession(game, in, os.Stdout)

	var parser *intent.Parser
	if *nl {
		target, err := resolveProvider(*provider, *model)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := ensurePrice(target.Model, *priceIn, *priceOut); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var cheap llm.Target
		// Ход, не меняющий мир, не обязан стоить как ход, который его меняет.
		// Без флага всё идёт основной моделью — молча дешеветь за счёт
		// качества нельзя.
		if *modelCheap != "" {
			cheap = llm.Target{Provider: target.Provider, Model: *modelCheap}
			if err := ensurePrice(cheap.Model, *priceInCheap, *priceOutCheap); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		router := buildRouter(target, cheap)
		gw := llm.NewGateway(router,
			llm.NewLedger(llm.Caps{
				GlobalDailyMicro:   int64(*capDay * 1_000_000),
				PerTurnCalls:       *capTurn,
				ForecastDailyMicro: int64(*capDay * 1_000_000 / 4),
			}, llm.WithAlert(func(spent, forecast int64) {
				fmt.Fprintf(os.Stderr, "расход %d мкд превысил прогноз %d вдвое\n", spent, forecast)
			})))
		if *debugLLM {
			gw = gw.WithDebug(os.Stderr)
		}
		parser = intent.NewParser(gw)
		session.WithInterpreter(&intent.GameInterpreter{Parser: parser, Game: game})
		act := actor.New(gw)
		if *guardLines {
			act = act.WithGuard(actor.NewGuard(gw))
		}
		gm := master.New(gw)
		session.WithVoicer(&actor.GameVoicer{
			Actor: act, Game: game, Turn: session.Turn, Master: gm,
			Notify: func(err error) {
				fmt.Fprintf(os.Stderr, "(Мастер не ответил: %v)\n", err)
			}})
		session.WithNarrator(&narrator{master: gm, game: game})
		defer func() { reportMetrics(gw, parser) }()
	}

	if err := session.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// appRoles — роли, которыми игра пользуется, и тир, которым каждая
// спрашивает. Список объявлен рядом с проводкой не для красоты: роль,
// которой пользуются, но которую забыли зароутить, падает в ErrNoProvider, а
// откат надстройки молчит — и это уже однажды выключило прозу Мастера на
// целую фазу, не сломав ни одного теста.
var appRoles = []struct {
	Role llm.Role
	Tier llm.Tier
}{
	{llm.RoleIntentParser, llm.TierMain},
	{llm.RoleActor, llm.TierMain},
	{llm.RoleActor, llm.TierCheap},    // приветствие, прощание, ремонт реплики
	{llm.RoleNarrator, llm.TierMain},  // проза сцены и исхода
	{llm.RoleNarrator, llm.TierCheap}, // решение ambient-детали по запросу
	{llm.RoleCanonGuard, llm.TierMain},
}

// buildRouter разводит роли по целям. Пустая дешёвая цель означает «всё
// основной моделью»: тир тогда не разводится, и фолбэк шлюза сам отдаёт
// основную цель.
func buildRouter(target, cheap llm.Target) *llm.Router {
	router := llm.NewRouter().
		Route(llm.RoleIntentParser, target).
		Route(llm.RoleActor, target).
		Route(llm.RoleNarrator, target).
		Route(llm.RoleCanonGuard, target)
	if cheap.Provider != nil && cheap.Model != "" {
		// Проверка реплики дешёвого тира не просит и потому по тирам не
		// разводится: дешёвый судья пропускает утечки.
		router.RouteCheap(llm.RoleActor, cheap).
			RouteCheap(llm.RoleNarrator, cheap)
	}
	return router
}

// narrator связывает Мастера с презентацией. Проза необязательна: без -nl
// печатается авторский текст, и это тот же текст, что служит Мастеру рамкой.
type narrator struct {
	master *master.Master
	game   *core.Game
}

func (n *narrator) Narrate(ctx context.Context, frame string, scene, outcome []string) (string, error) {
	return n.master.Narrate(ctx, frame,
		master.World{Setting: n.game.Setting, Scene: scene}, outcome, llm.Request{})
}

// reportMetrics печатает то, без чего слой моделей нельзя вести: расход,
// стоимость бита и долю непонятого ввода по классу глагола.
func reportMetrics(gw *llm.Gateway, p *intent.Parser) {
	s := gw.Stats()
	fmt.Fprintf(os.Stderr, "\nрасход: %.4f $  битов: %d\n",
		float64(s.SpentMicro)/1e6, s.Bits)
	for _, role := range []llm.Role{llm.RoleIntentParser, llm.RoleActor, llm.RoleNarrator} {
		if n := s.CallsByRole[role]; n > 0 {
			fmt.Fprintf(os.Stderr, "вызовов %s: %d (%.4f $)\n",
				role, n, float64(s.ByRole[role])/1e6)
		}
	}
	if cheap := s.CallsByTier[llm.TierCheap]; cheap > 0 {
		fmt.Fprintf(os.Stderr, "дешёвым тиром: %d вызовов (%.4f $), основным: %d (%.4f $)\n",
			cheap, float64(s.ByTier[llm.TierCheap])/1e6,
			s.CallsByTier[llm.TierMain], float64(s.ByTier[llm.TierMain])/1e6)
	}
	m := p.Metrics()
	if n := m.Observations(); n == 0 {
		fmt.Fprintln(os.Stderr, "свободный текст ни разу не разбирался")
	} else {
		fmt.Fprintf(os.Stderr, "непонятого ввода: %.0f%% из %d\n", m.Overall()*100, n)
	}
	if breached := m.Breaches(0.25); len(breached) > 0 {
		fmt.Fprintf(os.Stderr, "словарь узок в классах: %v\n", breached)
	}
}

// resolveProvider выбирает поставщика и модель. Слаги моделей у OpenRouter
// пространственные (vendor/model) и меняются, поэтому подставлять их по
// памяти нельзя — модель требуется указать явно.
func resolveProvider(name, model string) (llm.Target, error) {
	switch name {
	case "openrouter":
		if model == "" {
			return llm.Target{}, fmt.Errorf(
				"для -provider openrouter укажите -model, например anthropic/claude-sonnet-4.5\n" +
					"список слагов: https://openrouter.ai/models\n" +
					"ключ: переменная окружения OPENROUTER_API_KEY")
		}
		return llm.Target{Provider: llm.NewOpenRouter(llm.ORWithTitle("dnd")), Model: model}, nil
	case "anthropic":
		if model == "" {
			model = "claude-opus-5"
		}
		return llm.Target{Provider: llm.NewAnthropic(), Model: model}, nil
	default:
		return llm.Target{}, fmt.Errorf("неизвестный поставщик %q: ожидается openrouter или anthropic", name)
	}
}
