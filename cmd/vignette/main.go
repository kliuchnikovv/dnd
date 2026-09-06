// Command vignette — интерактивный CLI виньетки: сесть и сыграть сцену по ходам
// в терминале ТЕМ ЖЕ turn-путём, что и сервер: judge.Rule → State.Adjudicate →
// vignette.Narrate (Мастеру уходит ТОЛЬКО revealed) → страж. Не WS-клиент, а
// in-process консоль (как прото, но на доменном стеке) — чтобы калибровать и
// гонять живой Ф6. Каждый прогон пишется в runs/*.jsonl (vignette.Recorder) для
// офлайн-оценки утечек.
//
// Офлайн по умолчанию: KeywordJudge + склейка outcome (без ключа прозы нет) +
// KeywordGuard. С ключом богаче: LLMJudge + master.Master + генерация сцены.
// Три стартовые строки в stderr называют активные роли — fake-прогон не спутать
// с живым.
//
// Env (ДОМЕННАЯ конвенция, как cmd/server; НЕ прото per-role):
//
//	MODEL, PROVIDER (openrouter|anthropic), MODEL_CHEAP, CAP_DAY, CAP_TURN,
//	PRICE_IN/OUT[_CHEAP], OPENROUTER_API_KEY|ANTHROPIC_API_KEY — как у сервера.
//
// Специфика виньетки (аналога в домене нет — выбор сцены):
//
//	THEME  — задана → сгенерировать сцену (нужен ключ); пусто → фикстура
//	MODE   — traverse|disable|hold (для генерации; дефолт hold)
//	CASE   — фикстура-дело (дефолт nightguest)
//	CASES_ROOT — корень дел (дефолт "cases"; запускать из корня репо)
//	SEED   — зерно кости (дефолт 1)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/scenegen"
	"github.com/kliuchnikovv/dnd/vignette"
)

func main() {
	ctx := context.Background()
	seed := envInt64("SEED", 1)

	// Роли по наличию ключа. Живой режим — только когда есть И MODEL, И ключ
	// провайдера; иначе офлайн (fake-прогон не должен маскироваться под живой).
	// Ключ проверяем ДО сборки шлюза: без него незачем валидировать цену модели.
	var gw *llm.Gateway
	online := os.Getenv("MODEL") != "" && hasAPIKey(env("PROVIDER", "openrouter"))
	switch {
	case online:
		g, err := buildGateway()
		if err != nil {
			fatal(err)
		}
		gw = g
	case os.Getenv("MODEL") != "":
		fmt.Fprintln(os.Stderr, "[vignette] MODEL задан, но ключ провайдера не найден — играю офлайн")
	}

	// Сцена: живой режим + THEME → генерация (с фолбэком на фикстуру); иначе
	// фикстура cases/<CASE>/scene.json.
	sc, trace, srcNote := chooseScene(ctx, gw, online)
	st := vignette.NewState(dice.NewSource(seed).Stream("resolve"))

	// Роли.
	var (
		judge     vignette.Judge = vignette.KeywordJudge{}
		guard     vignette.Guard = vignette.KeywordGuard{}
		narr      vignette.Narrator
		judgeName = "KeywordJudge"
		narrName  = "нет (офлайн)"
	)
	if online {
		judge = vignette.NewLLMJudge(gw) // сам откатывается на keyword при сбое
		judgeName = "LLMJudge"
		narr = masterNarrator{master.New(gw)}
		narrName = "master.Master"
	}

	// Запись прогона (с gen-трейсом, если генерилось).
	rec, recPath, closeRec := openRecorder(sc, trace)
	defer closeRec()

	// Три стартовые строки в stderr — чтобы fake-прогон не спутать с живым.
	fmt.Fprintf(os.Stderr, "[vignette] сцена: %q · mode=%s · source=%s\n", sc.Title, sc.Mode, srcNote)
	fmt.Fprintf(os.Stderr, "[vignette] роли: судья=%s · нарратор=%s · страж=keyword\n", judgeName, narrName)
	fmt.Fprintf(os.Stderr, "[vignette] запись: %s\n", recPath)

	// Интро.
	fmt.Println()
	fmt.Println(sc.Intro)
	fmt.Println(vignette.Position(sc, st))
	fmt.Println("— пиши, что делаешь. :q или «выход» — закончить —")

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for {
		fmt.Print("\n> ")
		if !in.Scan() {
			break // EOF (Ctrl-D)
		}
		text := strings.TrimSpace(in.Text())
		if text == "" {
			continue
		}
		if text == ":q" || strings.EqualFold(text, "выход") {
			break
		}

		// ТОТ ЖЕ порядок хода, что в server.vignetteRuntime.applyInput.
		st.Turn++
		ruling := judge.Rule(text, vignette.BuildJudgeView(sc, st))
		res := st.Adjudicate(sc, ruling)

		if res.RollLine != "" {
			fmt.Println("  " + res.RollLine)
		}
		prose, nerr := vignette.Narrate(ctx, sc, st, res, narr, guard)
		if nerr != nil {
			fmt.Fprintf(os.Stderr, "[vignette] проза хода не удалась: %v\n", nerr)
		}
		if prose != "" {
			fmt.Println(prose)
		}
		// Онлайн проза — перифраз Мастера, поэтому нейтральное положение полезно
		// показать отдельной строкой. Офлайн склейка уже содержит StateNote —
		// не дублируем.
		if online && res.StateNote != "" {
			fmt.Println("  [положение] " + res.StateNote)
		}
		rec.Turn(text, ruling, res, prose)

		if res.Ended {
			fmt.Println("\n— занавес —")
			break
		}
	}
	if err := in.Err(); err != nil {
		fatal(fmt.Errorf("чтение ввода: %w", err))
	}
}

// masterNarrator адаптирует *master.Master к vignette.Narrator: строит World из
// нейтральных поверхностей и зовёт Narrate с ТОЛЬКО revealed (анти-лик ADR-0008).
// Тонкая glue-обёртка, не turn-логика — та единым кодом в vignette.Narrate.
type masterNarrator struct{ m *master.Master }

func (mn masterNarrator) Narrate(ctx context.Context, ambient string, surfaces, revealed []string, frame string) (string, error) {
	return mn.m.Narrate(ctx, master.KindOutcome, frame,
		master.World{Setting: ambient, Scene: surfaces}, revealed, "", nil, llm.Request{})
}

// chooseScene: живой режим + THEME → scenegen.Generate → Repair → Validate →
// FromSpec (фолбэк на фикстуру при любом сбое, как в прототипе); иначе фикстура.
func chooseScene(ctx context.Context, gw *llm.Gateway, online bool) (*vignette.Scene, *scenegen.GenTrace, string) {
	theme := os.Getenv("THEME")
	if online && theme != "" {
		mode := env("MODE", "hold")
		spec, tr, gerr := scenegen.Generate(ctx, gw, theme, mode)
		switch {
		case gerr != nil:
			fmt.Fprintf(os.Stderr, "[vignette] генерация сорвалась (%v) — фолбэк на фикстуру\n", gerr)
		default:
			scenegen.Repair(spec)
			if errs := scenegen.Validate(spec); len(errs) > 0 {
				fmt.Fprintf(os.Stderr, "[vignette] сгенерированная сцена невалидна (%s) — фолбэк\n", strings.Join(errs, "; "))
			} else {
				return vignette.FromSpec(spec), tr, fmt.Sprintf("generated THEME=%q MODE=%s", theme, mode)
			}
		}
		sc, note := loadFixtureOrDie()
		return sc, tr, note // трейс сохраняем даже при фолбэке — виден сбой генерации
	}
	sc, note := loadFixtureOrDie()
	return sc, nil, note
}

// loadFixtureOrDie грузит cases/<CASE>/scene.json (DTO scenegen.SceneSpec) —
// единый источник, та же валидная фикстура, что гоняет e2e.
func loadFixtureOrDie() (*vignette.Scene, string) {
	root := env("CASES_ROOT", "cases")
	name := env("CASE", "nightguest")
	path := filepath.Join(root, name, "scene.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		fatal(fmt.Errorf("фикстура %s: %w (запусти из корня репо или задай CASES_ROOT)", path, err))
	}
	var spec scenegen.SceneSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		fatal(fmt.Errorf("%s не разобралась: %w", path, err))
	}
	scenegen.Repair(&spec)
	if errs := scenegen.Validate(&spec); len(errs) > 0 {
		fatal(fmt.Errorf("%s невалидна: %s", path, strings.Join(errs, "; ")))
	}
	return vignette.FromSpec(&spec), fmt.Sprintf("fixture CASE=%s", name)
}

// openRecorder открывает runs/<ts>-<title>.jsonl и пишет gen-трейс (если сцена
// генерилась) первой строкой, затем шапку-ключ рекордера. rec может быть nil —
// Recorder.Turn это переносит.
func openRecorder(sc *vignette.Scene, trace *scenegen.GenTrace) (*vignette.Recorder, string, func()) {
	if err := os.MkdirAll("runs", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[vignette] запись отключена (%v)\n", err)
		return nil, "(нет)", func() {}
	}
	path := filepath.Join("runs", fmt.Sprintf("%s-%s.jsonl",
		time.Now().UTC().Format("20060102T150405"), slug(sc.Title)))
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[vignette] запись отключена (%v)\n", err)
		return nil, "(нет)", func() {}
	}
	if trace != nil {
		enc := json.NewEncoder(f)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(map[string]any{"type": "gen_trace", "trace": trace})
	}
	return vignette.NewRecorder(f, sc), path, func() { _ = f.Close() }
}

// slug — короткий безопасный кусок имени файла из заголовка сцены.
func slug(s string) string {
	out := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, s)
	out = strings.Trim(out, "-")
	if len(out) > 40 {
		out = out[:40]
	}
	if out == "" {
		return "scene"
	}
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "[vignette] "+err.Error())
	os.Exit(1)
}
