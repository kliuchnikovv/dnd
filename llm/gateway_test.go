package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func fixedClock(day string) func() time.Time {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}
	return func() time.Time { return t }
}

func gw(t *testing.T, caps Caps, opts ...LedgerOption) (*Gateway, *Fake) {
	t.Helper()
	p := NewFake("fake", true)
	r := NewRouter().
		Route(RoleNarrator, Target{p, "claude-sonnet-5"}).
		Route(RoleIntentParser, Target{p, "claude-haiku-4-5"})
	opts = append([]LedgerOption{WithClock(fixedClock("2026-08-18"))}, opts...)
	return NewGateway(r, NewLedger(caps, opts...)), p
}

func TestRoutesByRole(t *testing.T) {
	g, p := gw(t, Caps{})
	want := map[Role]string{
		RoleNarrator:     "claude-sonnet-5",
		RoleIntentParser: "claude-haiku-4-5",
	}
	for role, model := range want {
		resp, err := g.Do(context.Background(), Request{Role: role, Input: "текст"})
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if resp.Provider != "fake" {
			t.Errorf("%s: провайдер %q", role, resp.Provider)
		}
		if resp.Model != model {
			t.Errorf("%s ушла на модель %q, ожидалась %q", role, resp.Model, model)
		}
	}
	if n := len(p.Calls()); n != len(want) {
		t.Errorf("вызовов %d, ожидалось %d", n, len(want))
	}
}

// Правило из §11.3: выход парсера доходит до состояния, поэтому провайдер без
// гарантии схемы к этой роли не допускается — даже если он объявлен.
func TestProposingRoleRefusesNonStrictProvider(t *testing.T) {
	loose := NewFake("loose", false)
	r := NewRouter().Route(RoleIntentParser, Target{loose, "claude-haiku-4-5"})
	g := NewGateway(r, NewLedger(Caps{}, WithClock(fixedClock("2026-08-18"))))

	_, err := g.Do(context.Background(), Request{Role: RoleIntentParser, Input: "бью орка"})
	if !errors.Is(err, ErrSchemaUnsafe) {
		t.Fatalf("ошибка %v, ожидалась ErrSchemaUnsafe", err)
	}
	if len(loose.Calls()) != 0 {
		t.Error("провайдер без гарантии схемы был вызван")
	}
}

// Роль, ничего ядру не предлагающая, нестрогого провайдера принимает: её
// выход — вердикт-совет, и плохой разбор стоит бледной реплики, а не канона.
// Раньше здесь стоял нарратор, потом актёр; оба по реестру теперь что-то
// предлагают и строгого провайдера требуют, поэтому пример взят из судьи —
// его выход состояния не касается вовсе.
func TestNonProposingRoleAcceptsNonStrictProvider(t *testing.T) {
	loose := NewFake("loose", false)
	r := NewRouter().Route(RoleCanonGuard, Target{loose, "claude-sonnet-5"})
	g := NewGateway(r, NewLedger(Caps{}, WithClock(fixedClock("2026-08-18"))))

	if _, err := g.Do(context.Background(), Request{Role: RoleCanonGuard, Input: "сцена"}); err != nil {
		t.Fatalf("судья отвергнут: %v", err)
	}
}

func TestUnroutedRoleFails(t *testing.T) {
	g, _ := gw(t, Caps{})
	_, err := g.Do(context.Background(), Request{Role: RoleWorldsmith, Input: "x"})
	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("ошибка %v, ожидалась ErrNoProvider", err)
	}
}

func TestFallsBackOnProviderError(t *testing.T) {
	bad := NewFake("bad", true).FailWith(errors.New("503"))
	good := NewFake("good", true)
	r := NewRouter().Route(RoleNarrator,
		Target{bad, "claude-sonnet-5"}, Target{good, "claude-haiku-4-5"})
	g := NewGateway(r, NewLedger(Caps{}, WithClock(fixedClock("2026-08-18"))))

	resp, err := g.Do(context.Background(), Request{Role: RoleNarrator, Input: "сцена"})
	if err != nil {
		t.Fatalf("фолбэк не сработал: %v", err)
	}
	if resp.Provider != "good" {
		t.Errorf("ответ от %q, ожидался good", resp.Provider)
	}
	if len(bad.Calls()) != 1 {
		t.Error("основная цель не была попробована")
	}
}

func TestPerTurnCallCapStopsRegenerationLoop(t *testing.T) {
	g, p := gw(t, Caps{PerTurnCalls: 2})
	req := Request{Role: RoleNarrator, Input: "сцена", TurnID: "t1"}

	for i := 0; i < 2; i++ {
		if _, err := g.Do(context.Background(), req); err != nil {
			t.Fatalf("вызов %d отвергнут: %v", i+1, err)
		}
	}
	if _, err := g.Do(context.Background(), req); !errors.Is(err, ErrTurnCalls) {
		t.Fatalf("третий вызов дал %v, ожидалась ErrTurnCalls", err)
	}
	if len(p.Calls()) != 2 {
		t.Errorf("провайдер вызван %d раз, ожидалось 2 — отказ обязан быть бесплатным", len(p.Calls()))
	}
	// Другой ход не наказан за чужой цикл.
	if _, err := g.Do(context.Background(), Request{Role: RoleNarrator, Input: "x", TurnID: "t2"}); err != nil {
		t.Errorf("новый ход отвергнут: %v", err)
	}
}

func TestPerUserAndPerPartyCaps(t *testing.T) {
	g, _ := gw(t, Caps{PerUserDailyMicro: 1})
	req := Request{Role: RoleNarrator, Input: "длинная сцена на много токенов", UserID: "u1"}
	if _, err := g.Do(context.Background(), req); err != nil {
		t.Fatalf("первый вызов отвергнут: %v", err)
	}
	if _, err := g.Do(context.Background(), req); !errors.Is(err, ErrUserBudget) {
		t.Fatalf("второй вызов дал %v, ожидалась ErrUserBudget", err)
	}
	// Другой пользователь не задет.
	req.UserID = "u2"
	if _, err := g.Do(context.Background(), req); err != nil {
		t.Errorf("другой пользователь отвергнут: %v", err)
	}

	g2, _ := gw(t, Caps{PerPartyDailyMicro: 1})
	preq := Request{Role: RoleNarrator, Input: "длинная сцена на много токенов", PartyID: "p1"}
	if _, err := g2.Do(context.Background(), preq); err != nil {
		t.Fatal(err)
	}
	if _, err := g2.Do(context.Background(), preq); !errors.Is(err, ErrPartyBudget) {
		t.Fatalf("парти: %v, ожидалась ErrPartyBudget", err)
	}
}

// Глобальный потолок закрывает шлюз для всех: лучше час простоя, чем счёт
// на порядок больше прогноза.
func TestGlobalCapTripsKillSwitchForEveryone(t *testing.T) {
	g, _ := gw(t, Caps{GlobalDailyMicro: 1})
	if _, err := g.Do(context.Background(), Request{Role: RoleNarrator,
		Input: "длинная сцена на много токенов", UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	if !g.Stats().Killed {
		t.Fatal("шлюз не закрылся после превышения глобального потолка")
	}
	_, err := g.Do(context.Background(), Request{Role: RoleNarrator, Input: "x", UserID: "u2"})
	if !errors.Is(err, ErrKillSwitch) {
		t.Fatalf("другой пользователь получил %v, ожидалась ErrKillSwitch", err)
	}
}

func TestManualKillSwitch(t *testing.T) {
	p := NewFake("fake", true)
	l := NewLedger(Caps{}, WithClock(fixedClock("2026-08-18")))
	g := NewGateway(NewRouter().Route(RoleNarrator, Target{p, "claude-sonnet-5"}), l)
	l.Kill()
	if _, err := g.Do(context.Background(), Request{Role: RoleNarrator, Input: "x"}); !errors.Is(err, ErrKillSwitch) {
		t.Fatalf("рубильник не сработал: %v", err)
	}
}

func TestAlertFiresAtTwiceForecast(t *testing.T) {
	var got [2]int64
	fired := 0
	g, _ := gw(t, Caps{ForecastDailyMicro: 1},
		WithAlert(func(spent, forecast int64) { fired++; got = [2]int64{spent, forecast} }))
	for i := 0; i < 3; i++ {
		g.Do(context.Background(), Request{Role: RoleNarrator, Input: "длинная сцена на много токенов"})
	}
	if fired != 1 {
		t.Fatalf("алерт сработал %d раз, ожидался ровно 1", fired)
	}
	if got[0] < 2*got[1] {
		t.Errorf("алерт при расходе %d против прогноза %d", got[0], got[1])
	}
}

func TestDayRolloverResetsBudgetsAndKillSwitch(t *testing.T) {
	day := "2026-08-18"
	p := NewFake("fake", true)
	l := NewLedger(Caps{GlobalDailyMicro: 1}, WithClock(func() time.Time {
		t0, _ := time.Parse("2006-01-02", day)
		return t0
	}))
	g := NewGateway(NewRouter().Route(RoleNarrator, Target{p, "claude-sonnet-5"}), l)

	g.Do(context.Background(), Request{Role: RoleNarrator, Input: "длинная сцена на много токенов"})
	if !g.Stats().Killed {
		t.Fatal("шлюз должен был закрыться")
	}
	day = "2026-08-19"
	if _, err := g.Do(context.Background(), Request{Role: RoleNarrator, Input: "сцена"}); err != nil {
		t.Fatalf("новые сутки не сняли потолок: %v", err)
	}
	// Потолок в один микродоллар взводит рубильник на любом вызове, поэтому
	// проверять надо не Killed, а что счётчики суток обнулились.
	if s := g.Stats(); s.Bits != 1 {
		t.Errorf("битов после смены суток %d, ожидался 1 — счётчик не обнулился", s.Bits)
	}
}

func TestCostAccountingIsExact(t *testing.T) {
	// 1000 входных и 200 выходных токенов на sonnet: 1000*3 + 200*15 микро.
	u := Usage{InputTokens: 1000, OutputTokens: 200}
	cost, err := CostMicro("claude-sonnet-5", u)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(1000*3 + 200*15); cost != want {
		t.Errorf("стоимость %d, ожидалось %d микродолларов", cost, want)
	}
	if _, err := CostMicro("нет-такой-модели", u); !errors.Is(err, ErrUnknownModel) {
		t.Errorf("модель без цены дала %v, ожидалась ErrUnknownModel", err)
	}
}

// Бит — вызов полного нарратора; в нём считается вся юнит-экономика.
func TestCostPerBitCountsNarratorCallsOnly(t *testing.T) {
	g, _ := gw(t, Caps{})
	ctx := context.Background()
	g.Do(ctx, Request{Role: RoleNarrator, Input: "сцена один"})
	g.Do(ctx, Request{Role: RoleNarrator, Input: "сцена два"})
	g.Do(ctx, Request{Role: RoleIntentParser, Input: "бью орка"})

	s := g.Stats()
	if s.Bits != 2 {
		t.Errorf("битов %d, ожидалось 2 — парсер битом не является", s.Bits)
	}
	if s.CostPerBitMicro != s.SpentMicro/2 {
		t.Errorf("стоимость бита %d при расходе %d", s.CostPerBitMicro, s.SpentMicro)
	}
	if len(s.ByRole) != 2 {
		t.Errorf("расход разбит на %d ролей, ожидалось 2", len(s.ByRole))
	}
}

func TestUnknownModelPriceDoesNotSilentlySpend(t *testing.T) {
	p := NewFake("fake", true)
	g := NewGateway(NewRouter().Route(RoleNarrator, Target{p, "модель-без-цены"}),
		NewLedger(Caps{}, WithClock(fixedClock("2026-08-18"))))
	if _, err := g.Do(context.Background(), Request{Role: RoleNarrator, Input: "x"}); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("ошибка %v, ожидалась ErrUnknownModel", err)
	}
	if g.Stats().SpentMicro != 0 {
		t.Error("расход учтён по модели без цены")
	}
}

// Потолок «на ход» бесполезен, если ход не опознан. Адаптеры не заполняют
// TurnID, поэтому шлюз обязан брать его из контекста.
func TestTurnIDComesFromContext(t *testing.T) {
	g, p := gw(t, Caps{PerTurnCalls: 2})
	ctx := WithTurnID(context.Background(), "turn-1")
	req := Request{Role: RoleNarrator, Input: "сцена"} // TurnID не задан

	for i := 0; i < 2; i++ {
		if _, err := g.Do(ctx, req); err != nil {
			t.Fatalf("вызов %d: %v", i+1, err)
		}
	}
	if _, err := g.Do(ctx, req); !errors.Is(err, ErrTurnCalls) {
		t.Fatalf("третий вызов того же хода дал %v, ожидалась ErrTurnCalls", err)
	}
	if len(p.Calls()) != 2 {
		t.Errorf("провайдер вызван %d раз", len(p.Calls()))
	}
	// Следующий ход начинается с чистого счёта.
	if _, err := g.Do(WithTurnID(context.Background(), "turn-2"), req); err != nil {
		t.Errorf("новый ход отвергнут: %v", err)
	}
}

func TestExplicitTurnIDWinsOverContext(t *testing.T) {
	g, _ := gw(t, Caps{PerTurnCalls: 1})
	ctx := WithTurnID(context.Background(), "turn-1")
	if _, err := g.Do(ctx, Request{Role: RoleNarrator, Input: "x", TurnID: "own"}); err != nil {
		t.Fatal(err)
	}
	// Ход из контекста не тронут, потому что запрос назвал свой.
	if _, err := g.Do(ctx, Request{Role: RoleNarrator, Input: "x"}); err != nil {
		t.Errorf("ход из контекста задет чужим счётчиком: %v", err)
	}
}

// Два вызова парсера на один ввод означают сработавший раунд починки. Увидеть
// это можно только по счёту вызовов: расход не различает один дорогой вызов
// от двух дешёвых.
func TestCallsByRoleCountedSeparatelyFromSpend(t *testing.T) {
	g, _ := gw(t, Caps{})
	ctx := context.Background()
	g.Do(ctx, Request{Role: RoleIntentParser, Input: "первая попытка"})
	g.Do(ctx, Request{Role: RoleIntentParser, Input: "починка"})
	g.Do(ctx, Request{Role: RoleNarrator, Input: "сцена"})

	s := g.Stats()
	if s.CallsByRole[RoleIntentParser] != 2 {
		t.Errorf("вызовов парсера %d, ожидалось 2", s.CallsByRole[RoleIntentParser])
	}
	if s.CallsByRole[RoleNarrator] != 1 {
		t.Errorf("вызовов нарратора %d, ожидался 1", s.CallsByRole[RoleNarrator])
	}
	if s.CallsByRole[RoleActor] != 0 {
		t.Error("роль без вызовов попала в счёт")
	}
}

// Слаги OpenRouter пространственные и в таблицу цен не попадают. Спросить про
// цену надо уметь до вызова: узнать о неизвестной модели из ошибки посреди
// сессии — значит потерять сессию.
func TestHasPriceAnswersBeforeTheCall(t *testing.T) {
	if !HasPrice("claude-opus-5") {
		t.Error("цена известной модели не найдена")
	}
	if HasPrice("anthropic/claude-sonnet-4.5") {
		t.Error("слаг без цены объявлен известным")
	}
	SetPrice("anthropic/claude-sonnet-4.5", Price{3_000_000, 15_000_000})
	if !HasPrice("anthropic/claude-sonnet-4.5") {
		t.Error("цена, заданная руками, не подхватилась")
	}
}

// Ход без механических последствий не должен стоить как ход, меняющий мир.
// Приветствие и прощание идут дешёвым тиром; если дешёвая цель для роли не
// объявлена, запрос честно уходит на основную, а не падает.
func TestCheapTierRoutesToItsOwnTarget(t *testing.T) {
	main := NewFake("main", true).ReplyWith(func(Request) string { return "основная" })
	cheap := NewFake("cheap", true).ReplyWith(func(Request) string { return "дешёвая" })
	r := NewRouter().
		Route(RoleActor, Target{main, "claude-opus-5"}).
		RouteCheap(RoleActor, Target{cheap, "claude-haiku-4-5"})
	g := NewGateway(r, NewLedger(Caps{GlobalDailyMicro: 1_000_000}))

	got, err := g.Do(context.Background(), Request{Role: RoleActor, Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "основная" {
		t.Errorf("обычный запрос ушёл не туда: %q", got.Text)
	}

	got, err = g.Do(context.Background(), Request{Role: RoleActor, Input: "x", Tier: TierCheap})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "дешёвая" {
		t.Errorf("дешёвый запрос ушёл не туда: %q", got.Text)
	}
}

func TestCheapTierFallsBackToMainWhenNotDeclared(t *testing.T) {
	main := NewFake("main", true).ReplyWith(func(Request) string { return "основная" })
	g := NewGateway(NewRouter().Route(RoleActor, Target{main, "claude-opus-5"}),
		NewLedger(Caps{GlobalDailyMicro: 1_000_000}))

	got, err := g.Do(context.Background(), Request{Role: RoleActor, Input: "x", Tier: TierCheap})
	if err != nil {
		t.Fatalf("дешёвый запрос без дешёвой цели упал: %v", err)
	}
	if got.Text != "основная" {
		t.Errorf("откат не сработал: %q", got.Text)
	}
}

// Метрика, ради которой тир и заводится: сколько ушло на ходы, ничего не
// менявшие в мире.
func TestSpendIsCountedPerTier(t *testing.T) {
	p := NewFake("p", true).ReplyWith(func(Request) string { return "ответ" })
	g := NewGateway(NewRouter().
		Route(RoleActor, Target{p, "claude-opus-5"}).
		RouteCheap(RoleActor, Target{p, "claude-haiku-4-5"}),
		NewLedger(Caps{GlobalDailyMicro: 10_000_000}))

	ctx := context.Background()
	g.Do(ctx, Request{Role: RoleActor, Input: "длинный ход, меняющий мир"})
	g.Do(ctx, Request{Role: RoleActor, Input: "длинный ход, меняющий мир", Tier: TierCheap})

	s := g.Stats()
	if s.ByTier[TierCheap] == 0 {
		t.Error("расход дешёвого тира не посчитан")
	}
	if s.ByTier[TierCheap] >= s.ByTier[TierMain] {
		t.Errorf("дешёвый тир обошёлся не дешевле: %d против %d",
			s.ByTier[TierCheap], s.ByTier[TierMain])
	}
}

// countingWriter считает, сколько раз вызван Write. tui.Ring устроен так,
// что одна запись кольца — это один вызов Write: если дамп бьётся на
// несколько Fprintf, одна запись кольца превращается в фрагмент обмена, а не
// в обмен целиком, и потолок в записях врёт о числе реальных обменов.
type countingWriter struct {
	writes int
	buf    strings.Builder
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.writes++
	return c.buf.Write(p)
}

// Дамп обмена обязан уходить ОДНИМ вызовом Write — иначе одна «запись»
// кольца отладки — это фрагмент обмена, а не обмен целиком (см. tui.Ring).
func TestDumpWritesExchangeInOneCall(t *testing.T) {
	g, _ := gw(t, Caps{})
	w := &countingWriter{}
	g.WithDebug(w)

	if _, err := g.Do(context.Background(), Request{
		Role: RoleNarrator, Input: "текст", Schema: "{}"}); err != nil {
		t.Fatalf("Do: %v", err)
	}

	if w.writes != 1 {
		t.Errorf("дамп обмена ушёл %d вызовами Write, ожидался 1", w.writes)
	}
	if !strings.Contains(w.buf.String(), "схема: {}") {
		t.Errorf("схема пропала из дампа: %q", w.buf.String())
	}
}
