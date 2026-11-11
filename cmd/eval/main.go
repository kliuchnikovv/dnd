// Command eval — эвал ИИ-мастера по Эталону.
//
// Что делает: гонит ИИ-Мастера по ходам золотой катки «Тихий Дол»
// (docs/design/2026-09-02-master-reference-standard.md) и оценивает КАЖДЫЙ
// ход по всем 13 пунктам рубрики A–D. На выходе — Markdown-отчёт и JSONL.
//
// Запуск:
//
//	go run ./cmd/eval                       # золотой прогон против живой модели
//	go run ./cmd/eval -model anthropic/claude-sonnet-4.5
//	go run ./cmd/eval -turns 3              # первые три хода (быстрый смоук)
//	go run ./cmd/eval -dry                  # без модели: печать сценария + auto-пункты
//
// Ключ провайдера берётся из .env (OPENROUTER_API_KEY / ANTHROPIC_API_KEY).
// .env читает САМ бинарник: `source .env` задаёт переменную оболочки, а не
// окружения, и дочерний процесс её не видит. Уже заданное окружение
// перебивает файл — см. пакет dotenv.
//
// Почему рубрика оценивает СЫРУЮ прозу Мастера, а не после стража:
// D13 («Мастер знает секрет и дозирует его») — про Мастера. Если гард
// спрячет утечку, метрика ДНЕМАЕТ, и мы больше не знаем, держит ли Мастер
// середину. Поэтому по умолчанию страж выключен, а при -guard=check его
// вердикт печатается отдельной строкой: «в проде починил бы».
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kliuchnikovv/dnd/dotenv"
	"github.com/kliuchnikovv/dnd/eval/rubric"
	"github.com/kliuchnikovv/dnd/guard"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
)

func main() {
	var (
		envPath   = flag.String("env", ".env", "путь к файлу с ключами провайдера")
		model     = flag.String("model", "", "модель (по умолчанию MODEL из окружения)")
		provider  = flag.String("provider", "", "провайдер: openrouter | anthropic (по умолчанию PROVIDER или openrouter)")
		priceIn   = flag.Float64("price-in", 0, "цена входа, $/млн токенов (0 — PRICE_IN или таблица llm/pricing.go)")
		priceOut  = flag.Float64("price-out", 0, "цена выхода, $/млн токенов (0 — PRICE_OUT или таблица llm/pricing.go)")
		dry       = flag.Bool("dry", false, "без модели: напечатать сценарий и прогнать только структурные пункты")
		stub      = flag.Bool("stub", false, "прогон по проводке на llm.Fake: проверяет, что конвейер живой; оценки бессмысленны")
		guardMode = flag.String("guard", "off", "режим стража: off | check | enforce")
		out       = flag.String("out", "", "файл JSONL с результатами (пусто — не писать)")
		turnsN    = flag.Int("turns", 0, "ограничить число ходов (0 — вся катка)")
		quiet     = flag.Bool("quiet", false, "не печатать прозы в отчёт (только сводку и провалы)")
	)
	flag.Parse()

	// Ключ — из .env. Уже заданное окружение перебивает файл.
	dotenv.Load(*envPath)

	if err := run(*envPath, *model, *provider, *priceIn, *priceOut, *dry, *stub, *guardMode, *out, *turnsN, *quiet); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

func run(envPath, model, provider string, priceIn, priceOut float64, dry, stub bool, guardMode, out string, turnsN int, quiet bool) error {
	turns := rubric.GoldenRun()
	if turnsN > 0 && turnsN < len(turns) {
		turns = turns[:turnsN]
	}

	rub := rubric.NewRubric()

	// ── Режим без модели ────────────────────────────────────────────────
	// Печатает сценарий (что именно проверяем) и прогоняет auto-пункты по
	// ПУСТОЙ прозе. Смысл: проверить, что инструмент встаёт и видно, чего
	// ждать. Оценок мастера тут нет, и Report честно покажет это пустыми
	// вердиктами.
	if dry {
		return runDry(turns, rub)
	}

	// ── Модель ──────────────────────────────────────────────────────────
	// Режим stub: конвейер целиком на llm.Fake. Проверяет проводку (шлюз,
	// роли, рубрика, отчёт) без ключа и без денег. Оценки бессмысленны —
	// это НЕ замер качества Мастера.
	if stub {
		return runStub(turns, rub, out, quiet)
	}

	if model == "" {
		model = os.Getenv("MODEL")
	}
	if model == "" {
		return errors.New("модель не задана: укажи -model или MODEL в .env/окружении")
	}
	if provider == "" {
		provider = os.Getenv("PROVIDER")
	}
	if provider == "" {
		provider = "openrouter"
	}
	target, err := resolveProvider(provider, model)
	if err != nil {
		return err
	}
	// Цена модели: флаг > окружение > таблица llm/pricing.go. Молча тратить
	// по неизвестной цене нельзя — расход тогда не в счёт.
	if priceIn <= 0 {
		priceIn = envFloat("PRICE_IN", 0)
	}
	if priceOut <= 0 {
		priceOut = envFloat("PRICE_OUT", 0)
	}
	if err := ensurePrice(target.Model, priceIn, priceOut); err != nil {
		return err
	}

	capDay := envFloat("CAP_DAY", 1.0)
	router := llm.NewRouter().
		Route(llm.RoleNarrator, target).
		Route(llm.RoleCanonGuard, target).
		Route(llm.RoleRubricJudge, target)
	gw := llm.NewGateway(router, llm.NewLedger(llm.Caps{
		GlobalDailyMicro:   int64(capDay * 1_000_000),
		ForecastDailyMicro: int64(capDay * 1_000_000 / 4),
		// Прогон на 9 ходов. На ход уходит: 1 вызов Мастера + 9 вызовов
		// LLM-судьи (по одному на мягкий пункт) + 1 вызов стража (если
		// включён). Потолок по умолчанию должен это пропускать, иначе эвал
		// молча обрежется на середине хода.
		PerTurnCalls: int(envFloat("CAP_TURN", 16)),
	}))

	// Судья рубрики на том же шлюзе: мягкие пункты (A1, B5–C12) оценивает
	// модель по формулировкам Эталона.
	rub = rub.WithLLM(rubric.NewGatewayJudge(gw))

	// Мастер БЕЗ стража: рубрика оценивает его сырую прозу (см. док-комментарий
	// пакета main). Страж — отдельный инструмент проверки, см. guardMode.
	ms := master.New(gw)
	var checker *guard.Guard
	if guardMode != "off" {
		checker = guard.New(gw)
	}

	ctx := context.Background()
	rep := &Report{
		Model:     model,
		Provider:  provider,
		Scenario:  "«Тихий Дол» (золотой прогон Эталона)",
		StartedAt: time.Now(),
	}

	for i, turn := range turns {
		frame, canon := rubric.GoldenPrompt(turns, i)
		if frame == "" {
			return fmt.Errorf("ход %d: пустая рамка — сценарий битый", i+1)
		}
		// TurnID — чтобы потолок CAP_TURN считался НА ХОД, а не на прогон.
		// Ход = вызов Мастера + вызов судьи рубрики + (по желанию) страж.
		turnID := fmt.Sprintf("turn-%d", i+1)

		prose, err := ms.Narrate(ctx, master.KindOutcome, frame,
			master.World{
				Setting: rubric.GoldenSetting,
				Scene:   []string{turn.StateNote},
			},
			canon, "", nil, llm.Request{TurnID: turnID})
		if err != nil {
			return fmt.Errorf("ход %d: мастер: %w", i+1, explainProviderError(err))
		}

		// Страж: либо проверяет сырую прозу (check) и только сообщает, либо
		// пропускает её через себя (enforce) — тогда оценивается уже
		// отредактированная проза (как в проде).
		guardNote := ""
		if checker != nil {
			material := append([]string{frame, rubric.GoldenSetting}, canon...)
			material = append(material, turn.Outcome...)
			v, gerr := checker.Check(ctx, prose, material, turn.Revealed, llm.Request{TurnID: turnID})
			switch {
			case gerr != nil:
				guardNote = "ошибка стража: " + gerr.Error()
			case v.OK:
				guardNote = "ok"
			default:
				guardNote = "нарушение: " + v.What
				if v.Leak {
					guardNote = "утечка: " + v.What
				}
				if v.ContradictsState {
					guardNote += " (противоречит состоянию)"
				}
				if guardMode == "enforce" {
					// Прод-путь: починенная проза — единственная, что видит игрок.
					fixed, ferr := ms.Narrate(ctx, master.KindOutcome, frame,
						master.World{Setting: rubric.GoldenSetting, Scene: []string{turn.StateNote}},
						canon, "", turn.Revealed, llm.Request{TurnID: turnID})
					if ferr == nil && strings.TrimSpace(fixed) != "" {
						prose = fixed
						guardNote += " → проза отремонтирована"
					} else {
						guardNote += " → ремонт не удался, проза осталась прежней"
					}
				}
			}
		}

		rec := turn.ToRecord(prose)
		// Мягкие пункты (9 вызовов модели) считаем параллельно: та же цена,
		// в разы меньше ожидания.
		verdicts := rub.ApplyConcurrent(ctx, rec)

		rep.Turns = append(rep.Turns, TurnResult{
			Index:    i + 1,
			Input:    turn.Input,
			Note:     turn.Note,
			Prose:    prose,
			Guard:    guardNote,
			Etalon:   turn.EtalonTags,
			Verdicts: verdicts,
		})

		fmt.Fprintf(os.Stderr, "ход %d/%d: %s\n", i+1, len(turns), shortState(verdicts))
	}

	// ── Вывод ───────────────────────────────────────────────────────────
	return emit(rep, out, quiet)
}

// runStub — конвейер целиком на llm.Fake. Не замер качества: проверка того,
// что шлюз, роли, Мастер, рубрика и отчёт связаны между собой. Ключ не
// нужен, денег не стоит.
//
// Fake отвечает заведомо мусором (эхо рамки), поэтому soft-пункты примут
// «na» — и это честно видно в отчёте как пробелы.
func runStub(turns []rubric.GoldenTurn, rub *rubric.Rubric, out string, quiet bool) error {
	const stubModel = "stub-eval"
	llm.SetPrice(stubModel, llm.Price{InputMicroPerMTok: 1, OutputMicroPerMTok: 1})

	// Strict=true обязателен: RoleNarrator вносит предложения ядру, а
	// маршрутизатор пускает на такую роль только провайдера, гарантирующего
	// схему (ErrSchemaUnsafe иначе).
	fake := llm.NewFake("stub", true).ReplyWith(func(r llm.Request) string {
		// Роли-судьи ждут JSON; остальным достаточно текста.
		if r.Role == llm.RoleRubricJudge {
			return `{"outcome":"na","reason":"stub: оценки нет"}`
		}
		return "(stub) " + firstLine(r.Input, 120)
	})
	router := llm.NewRouter().
		Route(llm.RoleNarrator, llm.Target{Provider: fake, Model: stubModel}).
		Route(llm.RoleCanonGuard, llm.Target{Provider: fake, Model: stubModel}).
		Route(llm.RoleRubricJudge, llm.Target{Provider: fake, Model: stubModel})
	gw := llm.NewGateway(router, llm.NewLedger(llm.Caps{
		GlobalDailyMicro:   1_000_000,
		ForecastDailyMicro: 250_000,
		PerTurnCalls:       16,
	}))
	rub = rub.WithLLM(rubric.NewGatewayJudge(gw))
	ms := master.New(gw)

	ctx := context.Background()
	rep := &Report{
		Model:     stubModel + " (stub: оценки бессмысленны)",
		Provider:  "fake",
		Scenario:  "«Тихий Дол» (золотой прогон Эталона)",
		StartedAt: time.Now(),
	}
	for i, turn := range turns {
		frame, canon := rubric.GoldenPrompt(turns, i)
		if frame == "" {
			return fmt.Errorf("ход %d: пустая рамка — сценарий битый", i+1)
		}
		prose, err := ms.Narrate(ctx, master.KindOutcome, frame,
			master.World{Setting: rubric.GoldenSetting, Scene: []string{turn.StateNote}},
			canon, "", nil, llm.Request{TurnID: fmt.Sprintf("turn-%d", i+1)})
		if err != nil {
			return fmt.Errorf("ход %d: мастер: %w", i+1, err)
		}
		verdicts := rub.ApplyConcurrent(ctx, turn.ToRecord(prose))
		rep.Turns = append(rep.Turns, TurnResult{
			Index: i + 1, Input: turn.Input, Note: turn.Note, Prose: prose,
			Etalon: turn.EtalonTags, Verdicts: verdicts,
		})
		fmt.Fprintf(os.Stderr, "ход %d/%d: %s\n", i+1, len(turns), shortState(verdicts))
	}
	return emit(rep, out, quiet)
}

// emit — вывод отчёта: Markdown на stdout + (по желанию) JSONL в файл.
func emit(rep *Report, out string, quiet bool) error {
	md := Markdown(rep)
	if quiet {
		md = mdWithoutProse(rep)
	}
	fmt.Print(md)
	if out != "" {
		if err := writeJSONL(out, rep); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "JSONL: %s\n", out)
	}
	return nil
}

// explainProviderError — превращает ответ провайдера в actionable сообщение.
//
// Особый случай — 401 от OpenRouter: «User not found» при живом формате
// ключа означает, что аккаунт за ключом удалён/отозван (ключ читает
// /models, но чат не проходит). Это самая частая поломка эвала, и по
// сырому JSON её не увидеть — поэтому переводим словами.
func explainProviderError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "вернул 401"), strings.Contains(msg, "401"):
		return fmt.Errorf("%w\n"+
			"  ключ провайдера отвергнут. Проверь .env: OPENROUTER_API_KEY должен быть "+
			"живым ключом OpenRouter (sk-or-v1-...). Ошибка «User not found» при верном "+
			"формате ключа = аккаунт за ключом удалён или отозван. "+
			"До замены ключа прогонать конвейер можно через -stub", err)
	case strings.Contains(msg, "не задан"):
		return fmt.Errorf("%w\n"+
			"  ключ не найден. Положи OPENROUTER_API_KEY (или ANTHROPIC_API_KEY) в .env "+
			"или в окружение; путь к файлу меняется флагом -env", err)
	case strings.Contains(msg, "цена модели"):
		return fmt.Errorf("%w\n"+
			"  цена модели неизвестна — задай -price-in/-price-out ($/млн токенов)", err)
	}
	return err
}

// firstLine — первая строка текста, обрезанная до n символов. Для stub-эха.
func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n]) + "…"
	}
	return s
}

// runDry — режим без модели: печатает, что именно проверяется, и считает
// auto-пункты по пустой прозе. Полезно, чтобы заранее увидеть сценарий и
// убедиться, что инструмент встал.
func runDry(turns []rubric.GoldenTurn, rub *rubric.Rubric) error {
	fmt.Printf("# Сценарий золотого прогона «Тихий Дол» (dry-run, модели нет)\n\n")
	for i, turn := range turns {
		fmt.Printf("## Ход %d\n\n", i+1)
		if turn.Input != "" {
			fmt.Printf("**Заявка:** %s\n\n", turn.Input)
		}
		fmt.Printf("**Рамка для Мастера:**\n%s\n\n", turn.Frame)
		if len(turn.Outcome) > 0 {
			fmt.Printf("**Что открывается:**\n")
			for _, o := range turn.Outcome {
				fmt.Printf("- %s\n", o)
			}
			fmt.Println()
		}
		if turn.Roll != "" {
			fmt.Printf("**Бросок:** %s\n\n", turn.Roll)
		}
		if turn.NpcRoll != nil {
			fmt.Printf("**Бросок мира:** %s\n\n", turn.NpcRoll.Line)
		}
		// Применимость пунктов: структурная часть (по ходу) + мягкая (по
		// смыслу прозы, решает судья). Показываем честно: «не применим» и
		// «нужен судья» — разные вещи.
		rec := turn.ToRecord("")
		fmt.Printf("**Применимость пунктов:**\n\n")
		var structural, semantic []string
		for _, item := range rubric.Items() {
			switch rubric.ApplicabilityOf(item, rec) {
			case rubric.Applies:
				structural = append(structural, string(item))
			case rubric.NotApplicable:
				// молча пропускаем: неприменимые пункты в отчёте — «–»
			default:
				semantic = append(semantic, string(item))
			}
		}
		if len(structural) > 0 {
			fmt.Printf("- по структуре хода: %s\n", strings.Join(structural, ", "))
		}
		if len(semantic) > 0 {
			fmt.Printf("- по смыслу прозы (решает LLM-судья): %s\n", strings.Join(semantic, ", "))
		}
		if len(structural) == 0 && len(semantic) == 0 {
			fmt.Printf("- (ничего)\n")
		}
		fmt.Println()
	}
	return nil
}

// resolveProvider — выбор провайдера. Ключ провайдер читает сам (из окружения,
// куда его положил dotenv.Load).
func resolveProvider(name, model string) (llm.Target, error) {
	switch name {
	case "openrouter":
		return llm.Target{Provider: llm.NewOpenRouter(llm.ORWithTitle("dnd-eval")), Model: model}, nil
	case "anthropic":
		return llm.Target{Provider: llm.NewAnthropic(), Model: model}, nil
	default:
		return llm.Target{}, fmt.Errorf("неизвестный провайдер %q: ожидается openrouter или anthropic", name)
	}
}

// ensurePrice — регистрирует цену модели, если её нет в таблице. Молча
// тратить по неизвестной цене нельзя: расход тогда не в счёт.
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
		"($/млн токенов) или возьмите модель из таблицы llm/pricing.go", model)
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err == nil {
			return f
		}
	}
	return def
}

// writeJSONL — построчная выгрузка результатов.
func writeJSONL(path string, r *Report) error {
	data, err := JSONL(r)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// shortState — короткая сводка по ходу для stderr.
func shortState(vs []rubric.Verdict) string {
	var p, f, n int
	for _, v := range vs {
		switch v.Outcome {
		case rubric.OutcomePass:
			p++
		case rubric.OutcomeFail:
			f++
		default:
			n++
		}
	}
	return fmt.Sprintf("pass=%d fail=%d n/a=%d", p, f, n)
}
