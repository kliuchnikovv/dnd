package actor

import (
	"context"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func actorWith(t *testing.T, reply string) (*Actor, *llm.Fake) {
	t.Helper()
	f := llm.NewFake("fake", true).ReplyWith(func(llm.Request) string { return reply })
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleActor, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	return New(gw), f
}

func harbour(t *testing.T) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	return core.NewGame(*cfg)
}

// Модель иногда обрамляет реплику сама. Обрамление снимается здесь, а
// ставится в презентации — чтобы речь NPC и речь игрока выглядели одинаково.
func TestCleanStripsModelQuoting(t *testing.T) {
	for _, in := range []string{`Дождь скоро кончится.`, `«Дождь скоро кончится.»`,
		`"Дождь скоро кончится."`, `  Дождь скоро кончится.  `} {
		if got := clean(in); got != "Дождь скоро кончится." {
			t.Errorf("clean(%q) = %q", in, got)
		}
	}
}

func TestLineIsQuotedByCodeNotModel(t *testing.T) {
	a, _ := actorWith(t, `{"move":"deflect","line":"«Я ничего не видел.»"}`)
	got, err := a.Line(context.Background(), Speaker{Name: "Берн", Voice: "сухой"},
		Situation{Verb: "talk_to"}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Я ничего не видел." {
		t.Errorf("реплика %q — обрамление модели должно быть снято", got)
	}
}

// Персонаж не имеет права ссылаться на то, чего парти не знает, поэтому в
// промпт уходит ровно банк тем — и явное указание, что ссылаться больше не на что.
func TestPromptCarriesOnlyKnownTopics(t *testing.T) {
	g := harbour(t)
	a, f := actorWith(t, `{"move":"deflect","line":"Ничего."}`)
	sp, ok := SpeakerFor(g, "e_bern")
	if !ok {
		t.Fatal("у Берна нет голоса — фикстура сломана")
	}
	if _, err := a.Line(context.Background(), sp,
		Situation{Verb: "talk_to", Known: KnownTopics(g)}, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	prompt := f.Calls()[0].Input
	for _, k := range KnownTopics(g) {
		if !strings.Contains(prompt, k.Text) {
			t.Errorf("в промпте нет известной темы %q", k.Text)
		}
	}
	// Токены правильного ответа не должны попасть в промпт ни под каким видом.
	for _, leak := range []string{"seal_cord", "night_before_tide", "audit_shortfall", "redacted"} {
		if strings.Contains(prompt, leak) {
			t.Errorf("промпт содержит %q", leak)
		}
	}
	if !strings.Contains(f.Calls()[0].System, "Категорически запрещено") {
		t.Error("в системном промпте нет запрета на выдумку")
	}
}

func TestSpeakerOnlyForNPCsWithVoice(t *testing.T) {
	g := harbour(t)
	if _, ok := SpeakerFor(g, "e_body"); ok {
		t.Error("у предмета появился голос")
	}
	if _, ok := SpeakerFor(g, "e_нет_такого"); ok {
		t.Error("говорит несуществующая сущность")
	}
	sp, ok := SpeakerFor(g, "e_toke")
	if !ok || sp.Voice == "" || sp.Name == "" {
		t.Errorf("говорящий собран неверно: %+v", sp)
	}
}

func TestDispositionReachesPrompt(t *testing.T) {
	g := harbour(t)
	g.Disposition["e_bern"] = -3
	a, f := actorWith(t, `{"move":"refuse","line":"Отойдите."}`)
	sp, _ := SpeakerFor(g, "e_bern")
	a.Line(context.Background(), sp, Situation{Verb: "talk_to"}, llm.Request{})
	if !strings.Contains(f.Calls()[0].Input, "враждебное") {
		t.Errorf("расположение не доехало: %q", f.Calls()[0].Input)
	}
}

// --- политика: когда персонаж говорит ---

func voicer(t *testing.T, g *core.Game, reply string) (*GameVoicer, *llm.Fake) {
	t.Helper()
	a, f := actorWith(t, reply)
	return &GameVoicer{Actor: a, Game: g}, f
}

func TestVoiceOnSocialVerb(t *testing.T) {
	g := harbour(t)
	v, _ := voicer(t, g, `{"move":"smalltalk","line":"Мокро сегодня."}`)
	got, err := v.Voice(context.Background(),
		core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_bern"}}, core.TurnResult{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Мокро сегодня." {
		t.Errorf("реплика %q", got)
	}
}

// Ход, выдавший факт, уже несёт авторскую реплику: вторая была бы шумом.
func TestSilentWhenFactDelivered(t *testing.T) {
	g := harbour(t)
	v, f := voicer(t, g, `{"move":"deflect","line":"не должно прозвучать"}`)
	got, err := v.Voice(context.Background(),
		core.Intent{Verb: "question", Args: core.Args{Target: "e_bern", Topic: "f_x"}},
		core.TurnResult{Learned: []core.Learned{{Fact: "f_x", From: "e_bern"}}})
	if err != nil || got != "" {
		t.Errorf("персонаж заговорил поверх выдачи факта: %q %v", got, err)
	}
	if len(f.Calls()) != 0 {
		t.Error("модель вызвана впустую — это деньги за шум")
	}
}

func TestSilentOnNonSocialVerbsAndThings(t *testing.T) {
	g := harbour(t)
	cases := []struct {
		name string
		in   core.Intent
	}{
		{"осмотр предмета", core.Intent{Verb: "examine", Args: core.Args{Target: "e_body"}}},
		{"обыск", core.Intent{Verb: "search", Args: core.Args{Target: "e_bern"}}},
		{"без цели", core.Intent{Verb: "look"}},
		{"перемещение", core.Intent{Verb: "move_zone", Args: core.Args{Node: "n_forge"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, f := voicer(t, g, `{"move":"deflect","line":"тишина"}`)
			got, _ := v.Voice(context.Background(), c.in, core.TurnResult{})
			if got != "" {
				t.Errorf("прозвучала реплика: %q", got)
			}
			if len(f.Calls()) != 0 {
				t.Error("модель вызвана там, где говорить некому")
			}
		})
	}
}

// --- закрытый набор ходов ---

// Ход вне набора и подтверждение неизвестного факта — то, чем реплика
// превращалась в выдумку. Оба случая падают в шаблон.
func TestInvalidMoveFallsBackToTemplate(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	cases := map[string]string{
		"ход вне набора":             `{"move":"рассказать_всё","line":"Фермер Олсен потерял скот."}`,
		"подтверждение неизвестного": `{"move":"confirm_known","fact":"f_нет_такого","line":"Дежурства удвоили."}`,
		"факт при неподходящем ходе": `{"move":"deflect","fact":"f_body_found","line":"Патрулируем по графику."}`,
		"пустая реплика":             `{"move":"deflect","line":""}`,
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			a, _ := actorWith(t, reply)
			got, err := a.Line(context.Background(), sp,
				Situation{Verb: "talk_to", Known: KnownTopics(g)}, llm.Request{})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "Олсен") || strings.Contains(got, "Дежурства") ||
				strings.Contains(got, "Патрулируем") {
				t.Errorf("выдумка дошла до игрока: %q", got)
			}
			if got == "" {
				t.Error("шаблон не подставился")
			}
		})
	}
}

func TestConfirmKnownAcceptsFactFromMaterial(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	known := KnownTopics(g)
	if len(known) == 0 {
		t.Fatal("в деле нет стартовых фактов")
	}
	a, _ := actorWith(t, `{"move":"confirm_known","fact":"`+known[0].ID+
		`","line":"Тело нашли на складе, всё верно."}`)
	got, err := a.Line(context.Background(), sp,
		Situation{Verb: "talk_to", Known: known}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Тело нашли на складе, всё верно." {
		t.Errorf("реплика %q", got)
	}
}

// Слишком длинная реплика почти всегда означает рассказ о том, чего персонаж
// не знает.
func TestOverlongLineFallsBackToTemplate(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	long := strings.Repeat("много слов ", 40)
	a, _ := actorWith(t, `{"move":"smalltalk","line":"`+long+`"}`)
	got, _ := a.Line(context.Background(), sp, Situation{Verb: "talk_to"}, llm.Request{})
	if len([]rune(got)) > maxLine {
		t.Errorf("длинная реплика прошла: %d символов", len([]rune(got)))
	}
}

// Схема несёт материал: подтвердить можно только перечисленное.
func TestSchemaEnumeratesKnownFacts(t *testing.T) {
	g := harbour(t)
	props := schemaFor(KnownTopics(g))["properties"].(map[string]any)
	fact := props["fact"].(map[string]any)
	enum, ok := fact["enum"].([]string)
	if !ok || len(enum) == 0 {
		t.Fatal("у fact нет перечисления известных фактов")
	}
	moveEnum := props["move"].(map[string]any)["enum"].([]string)
	if len(moveEnum) != 5 {
		t.Errorf("ходов %d, ожидалось 5", len(moveEnum))
	}
}

// --- проверка на выдумку ---

type stubGuard struct {
	ok       bool
	err      error
	material []string
}

func (g *stubGuard) Check(_ context.Context, _ string, material []string, _ llm.Request) (bool, error) {
	g.material = material
	return g.ok, g.err
}

func TestGuardRejectionFallsBackToTemplate(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, _ := actorWith(t, `{"move":"smalltalk","line":"Дежурства удвоили с прошлой недели."}`)
	a = a.WithGuard(&stubGuard{ok: false})

	got, err := a.Line(context.Background(), sp, Situation{Verb: "talk_to"}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Дежурства") {
		t.Errorf("проверка отклонила реплику, а игрок её увидел: %q", got)
	}
}

// Сбой проверки трактуется как отказ: лучше бледно, чем с выдумкой.
func TestGuardFailureIsTreatedAsRejection(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, _ := actorWith(t, `{"move":"smalltalk","line":"Что-то содержательное."}`)
	a = a.WithGuard(&stubGuard{err: errStub})

	got, _ := a.Line(context.Background(), sp, Situation{Verb: "talk_to"}, llm.Request{})
	if got == "Что-то содержательное." {
		t.Error("реплика прошла, хотя проверка сорвалась")
	}
}

func TestGuardGetsOnlyAllowedMaterial(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	known := KnownTopics(g)
	sg := &stubGuard{ok: true}
	a, _ := actorWith(t, `{"move":"confirm_known","fact":"`+known[0].ID+`","line":"Тело нашли."}`)
	a = a.WithGuard(sg)
	a.Line(context.Background(), sp, Situation{Verb: "talk_to",
		PlayerText: "что за труп?", Known: known}, llm.Request{})

	joined := strings.Join(sg.material, " | ")
	if !strings.Contains(joined, known[0].Text) {
		t.Errorf("в материал не попал подтверждаемый факт: %q", joined)
	}
	for _, leak := range []string{"seal_cord", "night_before_tide", "redacted"} {
		if strings.Contains(joined, leak) {
			t.Errorf("в материал попало %q", leak)
		}
	}
}

var errStub = errStubType{}

type errStubType struct{}

func (errStubType) Error() string { return "проверка недоступна" }
