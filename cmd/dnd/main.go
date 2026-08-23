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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kliuchnikovv/dnd/actor"
	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/intent"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/tui"
	"golang.org/x/term"
)

func main() {
	casePath := flag.String("case", "cases/harbour/case.json", "путь к файлу дела")
	seed := flag.Int64("seed", 1, "seed RNG: прогон воспроизводится парой (seed, ввод)")
	script := flag.String("script", "", "файл команд вместо интерактивного ввода")
	journalPath := flag.String("journal", "",
		"писать журнал действий сессии в файл: команды до обработки, отметки о "+
			"применении, аудит недоверенного ввода")
	replayPath := flag.String("replay", "",
		"прогнать записанный журнал вместо ввода игрока; -case и -seed обязаны "+
			"совпадать с теми, на которых он снят")
	nl := flag.Bool("nl", false, "переводить свободный текст в действия через модель")
	chat := flag.Bool("chat", false,
		"диалоговый ввод: один вызов разбирает фразу и отвечает репликой Мастера, "+
			"после чего ядро решает, состоится ли ход")
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
	hunch := flag.Bool("hunch", false,
		"показывать подсказки чутья застрявшему игроку; пока выключено — "+
			"признак «застрял» считает холостым любой ход без находки, включая "+
			"спокойный осмотр мира")
	plain := flag.Bool("plain", false,
		"построчный режим на терминале: без полноэкранного окна, как в скрипте")
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

	if err := checkReplay(*replayPath, *script, *nl, *chat); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	raw, err := os.ReadFile(*casePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg, err := cases.Load(*casePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(*seed).Stream("resolve")
	// Снепшот сессии в прототипе — дело плюс seed: начальное состояние
	// выводится из них детерминированно. Хеш берётся от СОДЕРЖИМОГО дела, а не
	// от пути: правка дела делает журнал непереигрываемым, и узнать об этом
	// надо на реплее, а не по разъезжающимся фактам.
	snapshot := snapshotID(cfg.CaseID, raw, *seed)

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
	if *hunch {
		session.WithHunch()
	}

	var journal *cli.Journal
	if *journalPath != "" || *replayPath != "" {
		journal = cli.NewJournal(game.DB, sessionID(snapshot), snapshot, *seed)
		session.WithJournal(journal)
	}
	if *journalPath != "" {
		f, err := os.Create(*journalPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		journal.WithWriter(f)
	}
	if *replayPath != "" {
		if err := replaySession(session, *replayPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// fs решается один раз: и создание кольца отладки, и выбор драйвера ниже
	// обязаны видеть один и тот же ответ, иначе полноэкранный режим и его
	// собственный дамп рассинхронизируются на границе решения.
	fs := fullscreen(in, os.Stdout, *plain)

	var parser *intent.Parser
	var debugRing *tui.Ring
	var gw *llm.Gateway
	// noDebugReason — честный текст для панели отладки, когда полного дампа
	// нет: причина не всегда «не указан -debug-llm» (см. noDebugReasonFor).
	var noDebugReason string
	if err := checkModes(*nl, *chat); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if usesModels(*nl, *chat) {
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
		if fs {
			// В полноэкранном режиме и алерт леджера, и «Мастер не ответил»
			// обязаны уйти в то же кольцо, что и дамп -debug-llm, а не на
			// stderr: прямая печать поверх альт-экрана посреди хода корёжит
			// картинку. Кольцо заводится всегда, когда экран полноэкранный —
			// -debug-llm ниже только решает, попадёт ли туда и сам дамп.
			debugRing = tui.NewRing(200)
		}
		router := buildRouter(target, cheap)
		gw = llm.NewGateway(router,
			llm.NewLedger(llm.Caps{
				GlobalDailyMicro:   int64(*capDay * 1_000_000),
				PerTurnCalls:       *capTurn,
				ForecastDailyMicro: int64(*capDay * 1_000_000 / 4),
			}, llm.WithAlert(func(spent, forecast int64) {
				fmt.Fprintf(debugSink(debugRing), "расход %d мкд превысил прогноз %d вдвое\n", spent, forecast)
			})))
		if *debugLLM {
			if debugRing != nil {
				gw = gw.WithDebug(debugRing)
			} else {
				gw = gw.WithDebug(os.Stderr)
			}
		}
		// Разбор один и тот же; чат-режим отличается тем, что тем же вызовом
		// отвечает игроку, и подключается своим методом.
		if *chat {
			parser = intent.NewChatParser(gw)
			session.WithChat(&intent.GameInterpreter{Parser: parser, Game: game})
		} else {
			parser = intent.NewParser(gw)
			session.WithInterpreter(&intent.GameInterpreter{Parser: parser, Game: game})
		}
		act := actor.New(gw)
		if *guardLines {
			act = act.WithGuard(actor.NewGuard(gw))
		}
		// Почему реплика стала заглушкой — в ту же панель, что и остальные
		// внештатные сообщения. Игроку это не показывается: четыре пути к
		// заглушке дают одну фразу, и различать их нужно тому, кто отлаживает,
		// а не тому, кто играет.
		act = act.WithNotify(func(reason string) {
			fmt.Fprintf(debugSink(debugRing), "(%s)\n", reason)
		})
		gm := master.New(gw)
		session.WithVoicer(&actor.GameVoicer{
			Actor: act, Game: game, Turn: session.Turn, Master: gm,
			Notify: func(err error) {
				fmt.Fprintf(debugSink(debugRing), "(Мастер не ответил: %v)\n", err)
			}})
		voice := &narrator{master: gm, game: game, chat: *chat}
		session.WithNarrator(voice).WithRefuser(voice)
		defer func() { reportMetrics(gw, parser, session) }()
	}
	noDebugReason = noDebugReasonFor(usesModels(*nl, *chat), *debugLLM, debugRing)

	if fs {
		if err := tui.Run(session, tui.Options{
			Title:         filepath.Base(filepath.Dir(*casePath)),
			Status:        statusOf(game, session, gw),
			Debug:         debugRing,
			NoDebugReason: noDebugReason,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := session.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Поломка журнала — не деталь: на нём стоит воспроизводимость, и прогон,
	// который её потерял, обязан закончиться ненулевым кодом.
	if err := journal.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// snapshotID — идентификатор начального состояния: дело плюс seed. Читаемая
// часть оставлена нарочно: снепшот попадает в баг-репорт, и «harbour@a1b2c3d4»
// там полезнее одного хеша.
func snapshotID(caseID store.CaseID, content []byte, seed int64) string {
	h := sha256.New()
	h.Write(content)
	fmt.Fprintf(h, "|%d", seed)
	return fmt.Sprintf("%s@%s", caseID, hex.EncodeToString(h.Sum(nil))[:8])
}

// sessionID — какая это сессия. Два прогона одного дела на одном seed — две
// разные сессии, поэтому в идентификатор входит время запуска: журнал одной не
// должен молча дописываться в другую.
func sessionID(snapshot string) store.SessionID {
	return store.SessionID(fmt.Sprintf("%s/%s", snapshot,
		time.Now().UTC().Format("20060102T150405")))
}

// checkReplay отбивает реплей вместе с тем, что ему противоречит. Ввод игрока
// реплею не нужен, а модель в нём недетерминирована и стоит денег: молча
// проигнорировать любой из этих флагов значило бы выдать другой прогон за
// записанный.
func checkReplay(replay, script string, nl, chat bool) error {
	if replay == "" {
		return nil
	}
	if script != "" {
		return errors.New("-replay и -script вместе не работают: журнал сам " +
			"и есть ввод, а второй источник ходов сделал бы прогон не тем, что записан")
	}
	if nl || chat {
		return errors.New("-replay не работает с -nl и -chat: правда реплея — " +
			"записанный интент, а второй прогон через модель дал бы другой")
	}
	return nil
}

// replaySession прогоняет журнал с диска. Расхождение снепшота, seed или
// версии ядра ловит сам реплеер — здесь только чтение файла.
func replaySession(session *cli.Session, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dump, err := cli.ReadJournal(f)
	if err != nil {
		return err
	}
	return session.Replay(dump.Commands)
}

// checkModes отбивает -nl вместе с -chat. Молчаливый приоритет одного из них
// был бы худшим исходом: игрок получил бы разбор, о котором не просил, и не
// узнал бы об этом.
func checkModes(nl, chat bool) error {
	if nl && chat {
		return errors.New("-nl и -chat вместе не работают: это два разных разбора " +
			"одной фразы. -chat отвечает игроку тем же вызовом, которым разбирает")
	}
	return nil
}

// usesModels — нужен ли шлюз моделей. Оба режима свободного текста считаются
// одинаково: всё, что ниже зависит от шлюза (кольцо отладки, потолки, метрики,
// причина пустой панели), обязано видеть один ответ.
func usesModels(nl, chat bool) bool { return nl || chat }

// fullscreen сообщает, уместен ли полноэкранный режим. Только когда И ввод, И
// вывод — терминал: пайп, -script и тесты обязаны идти построчным путём,
// иначе воспроизводимость начнёт зависеть от способа запуска.
func fullscreen(in, out *os.File, plain bool) bool {
	if plain {
		return false
	}
	return isTerminal(in) && isTerminal(out)
}

// isTerminal — единственный честный способ ответить на этот вопрос. Проверка
// через os.ModeCharDevice отвечает "да" и на /dev/null: это тоже символьное
// устройство, и `dnd < /dev/null > /dev/null` уходил в полноэкранную ветку и
// падал на open /dev/tty. golang.org/x/term — официальный модуль Go, который
// умеет именно это: спросить у файлового дескриптора термиос, а не угадывать
// по типу файла.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// statusOf — строка шапки: место, ход, расход. Функция, а не строка, потому
// что все три меняются по ходу игры — там, где строка не устареет сама, ей
// самое место, а путь к файлу дела в шапке ни ходом, ни местом не меняется,
// и потому туда не идёт (см. Title в Run).
func statusOf(game *core.Game, session *cli.Session, gw *llm.Gateway) func() string {
	return func() string {
		place := game.DB.Locations[game.Node].Name
		status := fmt.Sprintf("%s · ход %d", place, session.Turn())
		if gw != nil {
			status += fmt.Sprintf(" · $%.4f", float64(gw.Stats().SpentMicro)/1e6)
		}
		return status
	}
}

// debugSink — куда уходят внештатные сообщения (алерт леджера, «Мастер не
// ответил»), которые срабатывают посреди хода, а не по запросу игрока. В
// построчном режиме им, как и раньше, место в stderr; в полноэкранном —
// в то же кольцо, что и дамп -debug-llm, иначе они бьют поверх альт-экрана.
func debugSink(ring *tui.Ring) io.Writer {
	if ring != nil {
		return ring
	}
	return os.Stderr
}

// noDebugReasonFor решает, что честно сказать панели отладки, когда полного
// дампа обмена в ней нет. Две разные причины, и путать их нельзя:
//
//   - -debug-llm дан без -nl: шлюза моделей вообще нет, кольцу неоткуда
//     взяться, и дамп невозможен в принципе.
//   - -nl дан без -debug-llm: кольцо есть (см. main выше — оно нужно как
//     приёмник алерта леджера и «Мастер не ответил» независимо от дампа),
//     но дампа в нём никогда не будет — без причины пустая панель читается
//     как поломка, а не как «дампа не просили».
func noDebugReasonFor(nl, debugLLM bool, ring *tui.Ring) string {
	switch {
	case !nl && debugLLM:
		return "-debug-llm без -nl или -chat ничего не даёт: моделей не вызывает ни один режим"
	case nl && !debugLLM && ring != nil:
		return "дамп обмена выключен: здесь только внештатные сообщения, " +
			"для полного дампа запустите с -debug-llm"
	default:
		return ""
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
	// Чат-режим: разбор и реплика одним вызовом. Основной тир и только он —
	// выход несёт интент, то есть мутирует состояние, и дешёвая модель тут
	// платила бы каноном за экономию.
	{llm.RoleChatMaster, llm.TierMain},
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
		Route(llm.RoleChatMaster, target).
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

// narrator связывает Мастера с презентацией — и прозой, и отказом. Обе
// надстройки необязательны: без -nl печатается авторский текст (он же служит
// Мастеру рамкой) и прежнее «нельзя: …». Голос один, потому что говорящий
// один: делить прозу и отказ между двумя объектами значило бы говорить с
// игроком двумя разными людьми.
type narrator struct {
	master *master.Master
	game   *core.Game
	// chat — чат-режим. Исход в нём Мастер не описывает: он уже ответил
	// игроку репликой ДО броска, и вторая проза о том же ходе — это пересказ
	// своими словами того, что игрок только что прочёл. Ход остаётся в один
	// вызов модели, то есть дешевле -nl. Описание МЕСТА оживляется по-прежнему:
	// это другая работа, и реплика её не заменяет.
	chat bool
}

func (n *narrator) Narrate(ctx context.Context, p cli.Prose) (string, error) {
	what := master.KindOutcome
	switch {
	case p.Kind == cli.ProseBriefing:
		what = master.KindBriefing
	case p.Kind == cli.ProsePlace:
		what = master.KindPlace
	case n.chat:
		// Пустой ответ означает откат на авторскую рамку — ровно то, что
		// нужно: исход печатает ядро. Брифинга это не касается: он не исход, и
		// оживить его — та же работа, что оживить место.
		return "", nil
	}
	return n.master.Narrate(ctx, what, p.Frame,
		master.World{Setting: n.game.Setting, Scene: p.Scene}, p.Outcome, p.Speaking, llm.Request{})
}

// Refuse произносит отказ мира. Тот же Мастер и тот же мир, что у прозы:
// отказ — такая же его реплика, как описание сцены, и делить их между двумя
// голосами значило бы говорить с игроком двумя разными людьми.
func (n *narrator) Refuse(ctx context.Context, refusal string) (string, error) {
	return n.master.Refuse(ctx, refusal,
		master.World{Setting: n.game.Setting, Scene: sceneFor(n.game)}, llm.Request{})
}

// sceneFor — что видно вокруг, словами для Мастера. Отказу сцена нужна затем
// же, зачем прозе: «здесь этого нет» произносится по-разному на пристани и в
// конторе гильдии.
func sceneFor(g *core.Game) []string {
	return []string{"Место: " + g.DB.Locations[g.Node].Name}
}

// reportMetrics печатает то, без чего слой моделей нельзя вести: расход,
// стоимость бита и долю непонятого ввода по классу глагола.
func reportMetrics(gw *llm.Gateway, p *intent.Parser, sess *cli.Session) {
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
	// Цена чат-режима: сколько раз модель ответила на ход, которого мир не
	// допускает. Высокая доля означает, что порядок «сначала ответить» неверен,
	// и это единственный способ узнать это, не споря.
	if shown, eaten := sess.ChatStats(); shown+eaten > 0 {
		fmt.Fprintf(os.Stderr, "реплик Мастера: %d показано, %d съел отказ ядра (%.0f%%)\n",
			shown, eaten, float64(eaten)/float64(shown+eaten)*100)
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
