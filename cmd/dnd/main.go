// Команда dnd — консольный прототип детективной игры.
//
// По умолчанию ни одного вызова модели: ввод структурированный, прогон
// воспроизводится парой (seed, ввод). Флаг -nl включает перевод свободного
// текста — тогда нераспознанный ввод уходит в модель, и воспроизводимость
// сохраняется только для структурированных команд.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/intent"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func main() {
	casePath := flag.String("case", "cases/harbour/case.json", "путь к файлу дела")
	seed := flag.Int64("seed", 1, "seed RNG: прогон воспроизводится парой (seed, ввод)")
	script := flag.String("script", "", "файл команд вместо интерактивного ввода")
	nl := flag.Bool("nl", false, "переводить свободный текст в действия через модель")
	model := flag.String("model", "claude-opus-5", "модель для перевода намерений")
	capDay := flag.Float64("cap-day", 1.0, "потолок расхода в долларах за сутки")
	capTurn := flag.Int("cap-turn", 3, "потолок вызовов модели на один ход")
	flag.Parse()

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
		gw := llm.NewGateway(
			llm.NewRouter().Route(llm.RoleIntentParser,
				llm.Target{Provider: llm.NewAnthropic(), Model: *model}),
			llm.NewLedger(llm.Caps{
				GlobalDailyMicro:   int64(*capDay * 1_000_000),
				PerTurnCalls:       *capTurn,
				ForecastDailyMicro: int64(*capDay * 1_000_000 / 4),
			}, llm.WithAlert(func(spent, forecast int64) {
				fmt.Fprintf(os.Stderr, "расход %d мкд превысил прогноз %d вдвое\n", spent, forecast)
			})))
		parser = intent.NewParser(gw)
		session.WithInterpreter(&intent.GameInterpreter{Parser: parser, Game: game})
		defer func() { reportMetrics(gw, parser) }()
	}

	if err := session.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// reportMetrics печатает то, без чего слой моделей нельзя вести: расход,
// стоимость бита и долю непонятого ввода по классу глагола.
func reportMetrics(gw *llm.Gateway, p *intent.Parser) {
	s := gw.Stats()
	fmt.Fprintf(os.Stderr, "\nрасход: %.4f $  битов: %d\n",
		float64(s.SpentMicro)/1e6, s.Bits)
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
