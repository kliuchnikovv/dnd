package actor

import (
	"context"
	"encoding/json"
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
	if !strings.Contains(f.Calls()[0].System, "Запрещено, и это проверяется") {
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
	g.D.Adjust("e_bern", -3)
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
	props := schemaFor(KnownTopics(g), nil, nil)["properties"].(map[string]any)
	fact := props["fact"].(map[string]any)
	enum, ok := fact["enum"].([]string)
	if !ok || len(enum) == 0 {
		t.Fatal("у fact нет перечисления известных фактов")
	}
	moveEnum := props["move"].(map[string]any)["enum"].([]string)
	if len(moveEnum) != len(moves()) {
		t.Errorf("ходов в схеме %d, в наборе %d", len(moveEnum), len(moves()))
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

// --- обстановка как разрешённый материал ---

// Без обстановки в материале проверка отвергала «мокро сегодня» как выдумку,
// и персонаж отвечал шаблоном. Стена из служебных формулировок — прямое
// следствие слишком узкого материала, а не характера.
func TestSceneIsAllowedMaterial(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	sg := &stubGuard{ok: true}
	a, _ := actorWith(t, `{"move":"observe","line":"Дождь третий день, вот и все новости."}`)
	a = a.WithGuard(sg)

	got, err := a.Line(context.Background(), sp, Situation{
		Verb: "talk_to", Known: KnownTopics(g), Scene: SceneOf(g, "e_bern"),
	}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Дождь третий день, вот и все новости." {
		t.Errorf("реплика об обстановке не прошла: %q", got)
	}
	joined := strings.Join(sg.material, " | ")
	if !strings.Contains(joined, "Пристань") {
		t.Errorf("в материал не попало место: %q", joined)
	}
	if !strings.Contains(joined, "Дождь") {
		t.Errorf("в материал не попало описание вокруг: %q", joined)
	}
}

func TestSceneOfCarriesPlaceWeatherAndCompany(t *testing.T) {
	g := harbour(t)
	scene := SceneOf(g, "e_bern")
	joined := strings.Join(scene, " | ")
	for _, want := range []string{"Место:", "Вокруг:", "Рядом:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("в обстановке нет %q: %q", want, joined)
		}
	}
	// Говорящий не считает себя за компанию.
	if strings.Contains(joined, "Берн") {
		t.Errorf("персонаж перечислен рядом с собой: %q", joined)
	}
	// И правда дела в обстановку не попадает.
	for _, leak := range []string{"seal_cord", "night_before_tide", "redacted"} {
		if strings.Contains(joined, leak) {
			t.Errorf("в обстановке %q", leak)
		}
	}
}

func TestObserveIsAValidMove(t *testing.T) {
	props := schemaFor(nil, nil, nil)["properties"].(map[string]any)
	enum := props["move"].(map[string]any)["enum"].([]string)
	// Каждый ход из набора обязан быть в схеме: ход, которого модель не видит,
	// существует только на бумаге.
	for _, want := range moves() {
		var found bool
		for _, m := range enum {
			if m == want {
				found = true
			}
		}
		if !found {
			t.Errorf("хода %q нет в схеме", want)
		}
	}
}

// Промпт обязан различать факты дела и обстановку: правила у них разные.
func TestPromptSeparatesFactsFromScene(t *testing.T) {
	g := harbour(t)
	a, f := actorWith(t, `{"move":"observe","line":"Мокро."}`)
	sp, _ := SpeakerFor(g, "e_bern")
	a.Line(context.Background(), sp, Situation{
		Verb: "talk_to", Known: KnownTopics(g), Scene: SceneOf(g, "e_bern"),
	}, llm.Request{})

	prompt := f.Calls()[0].Input
	if !strings.Contains(prompt, "Обстановка — об этом можно говорить свободно") {
		t.Errorf("обстановка не помечена как свободная: %q", prompt)
	}
	if !strings.Contains(prompt, "Факты дела — только это и можно подтверждать") {
		t.Errorf("факты не помечены как ограниченные: %q", prompt)
	}
}

// --- темы: не зачитывать и не повторять ---

// Тема, поднятая вне списка, отклоняется — иначе персонаж «упоминает» то,
// чего не знает.
func TestVolunteerRequiresTopicFromList(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, _ := actorWith(t, `{"move":"volunteer","topic":"t99","line":"А вот был случай."}`)
	got, err := a.Line(context.Background(), sp, Situation{
		Verb: "talk_to", Talks: topicsOf(g, "e_bern"), Scene: SceneOf(g, "e_bern"),
	}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got == "А вот был случай." {
		t.Error("принята тема вне списка")
	}
}

// Поднятая тема помечается рассказанной: второй раз она звучит как
// заклинивший автомат.
func TestVolunteeredTopicIsMarkedTold(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	before := topicsOf(g, "e_bern")
	if len(before) == 0 {
		t.Fatal("у Берна нет заметок — фикстура сломана")
	}
	a, _ := actorWith(t, `{"move":"volunteer","topic":"`+before[0].ID+
		`","line":"Книгу через пристань таскают, я докладывал."}`)

	v := &GameVoicer{Actor: a, Game: g}
	if _, err := v.Voice(context.Background(),
		core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_bern"}},
		core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	after := topicsOf(g, "e_bern")
	if len(after) != len(before)-1 {
		t.Errorf("заметок было %d, стало %d — рассказанное не вычитается",
			len(before), len(after))
	}
	_ = sp
}

// Заметки — не реплики. Дно для volunteer не пересказывает заметку дословно:
// дословная заметка звучит сводкой.
func TestVolunteerTemplateDoesNotReciteNote(t *testing.T) {
	g := harbour(t)
	talks := topicsOf(g, "e_bern")
	got := template(MoveVolunteer, Situation{Talks: talks})
	for _, tp := range talks {
		if got == tp.Note {
			t.Errorf("шаблон зачитал заметку дословно: %q", got)
		}
	}
}

// Промпт обязан требовать ответа на сказанное, а не выдачи известного.
func TestPromptDemandsAnsweringThePlayer(t *testing.T) {
	g := harbour(t)
	a, f := actorWith(t, `{"move":"smalltalk","line":"Здравствуйте."}`)
	sp, _ := SpeakerFor(g, "e_bern")
	a.Line(context.Background(), sp, Situation{Verb: "talk_to",
		Talks: topicsOf(g, "e_bern")}, llm.Request{})

	sys := f.Calls()[0].System
	if !strings.Contains(sys, "реплика ОТВЕЧАЕТ на то, что сказал игрок") {
		t.Error("в промпте нет требования отвечать на сказанное")
	}
	if !strings.Contains(sys, "не зачитывай") {
		t.Error("в промпте нет запрета зачитывать заметку")
	}
	if strings.Contains(sys, "Предпочитай") {
		t.Error("в промпте осталось предпочтение volunteer — оно и делало сводку")
	}
}

// --- акт игрока определяет набор ходов ---

func TestClassifyPlayerAct(t *testing.T) {
	cases := map[string]Act{
		"Поздороваться с Берном": ActGreeting,
		"привет": ActGreeting,
		"Что-нибудь слышно последнее время?": ActProbe,
		"Даже никаких слухов? Тухленько":     ActProbe,
		"что нового":            ActProbe,
		"а ты ничего не видел?": ActProbe,
		"А при чем тут дождь?":  ActPress,
		"почему":                ActPress,
		"спасибо":               ActThanks,
		"где склад?":            ActAsk,
		"я просто стою":         ActOther,
		"":                      ActOther,
	}
	for text, want := range cases {
		if got := Classify(text); got != want {
			t.Errorf("%q -> %q, ожидалось %q", text, got, want)
		}
	}
}

// Ровно тот провал: на открытый вопрос при непустых заметках отмолчаться
// нельзя — светской болтовни просто нет в наборе.
func TestProbeWithNotesForbidsSmalltalk(t *testing.T) {
	g := harbour(t)
	sit := Situation{Talks: topicsOf(g, "e_bern"), Scene: SceneOf(g, "e_bern")}
	allowed := movesFor(ActProbe, sit)

	for _, forbidden := range []Move{MoveSmalltalk, MoveObserve, MoveDeflect} {
		if allowedMove(forbidden, allowed) {
			t.Errorf("на открытый вопрос разрешён ход %q", forbidden)
		}
	}
	if !allowedMove(MoveVolunteer, allowed) {
		t.Error("на открытый вопрос не разрешено поделиться")
	}
}

// Без материала volunteer недоступен: иначе персонажа заставляют выдумывать.
func TestProbeWithoutNotesAllowsDeflection(t *testing.T) {
	allowed := movesFor(ActProbe, Situation{})
	if allowedMove(MoveVolunteer, allowed) {
		t.Error("нечего рассказать, а рассказать разрешено — это принуждение к выдумке")
	}
	if !allowedMove(MoveDeflect, allowed) {
		t.Error("без материала уклониться нельзя")
	}
}

// На приветствие сводки быть не должно — это и был первый перекос.
func TestGreetingForbidsVolunteering(t *testing.T) {
	g := harbour(t)
	allowed := movesFor(ActGreeting, Situation{Talks: topicsOf(g, "e_bern")})
	if allowedMove(MoveVolunteer, allowed) {
		t.Error("на приветствие разрешено вываливать известное")
	}
	if !allowedMove(MoveSmalltalk, allowed) {
		t.Error("на приветствие нельзя ответить любезностью")
	}
}

func TestConfirmKnownAvailableWheneverFactsExist(t *testing.T) {
	g := harbour(t)
	sit := Situation{Known: KnownTopics(g)}
	for _, act := range []Act{ActGreeting, ActProbe, ActPress, ActAsk, ActOther} {
		if !allowedMove(MoveConfirmKnown, movesFor(act, sit)) {
			t.Errorf("при акте %q нельзя подтвердить известный факт", act)
		}
	}
}

// Ход вне разрешённого набора не проходит: набор — это грамматика, а не совет.
func TestMoveOutsideAllowedSetIsRejected(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, _ := actorWith(t, `{"move":"smalltalk","line":"Погодка-то какая."}`)
	got, err := a.Line(context.Background(), sp, Situation{
		Verb: "talk_to", PlayerText: "что слышно?",
		Talks: topicsOf(g, "e_bern"), Scene: SceneOf(g, "e_bern"),
	}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got == "Погодка-то какая." {
		t.Error("болтовня прошла там, где игрок прямо спросил")
	}
}

// Схема отдаёт модели только разрешённые ходы: ход, которого нет в грамматике,
// выбрать невозможно.
func TestSchemaCarriesOnlyAllowedMoves(t *testing.T) {
	g := harbour(t)
	a, f := actorWith(t, `{"move":"volunteer","topic":"t1","line":"Книгу таскают без замка."}`)
	sp, _ := SpeakerFor(g, "e_bern")
	a.Line(context.Background(), sp, Situation{Verb: "talk_to",
		PlayerText: "что слышно?", Talks: topicsOf(g, "e_bern")}, llm.Request{})

	var probe map[string]any
	if err := json.Unmarshal([]byte(f.Calls()[0].Schema), &probe); err != nil {
		t.Fatal(err)
	}
	enum := probe["properties"].(map[string]any)["move"].(map[string]any)["enum"].([]any)
	for _, m := range enum {
		if m == string(MoveSmalltalk) {
			t.Error("в схеме на открытый вопрос осталась болтовня")
		}
	}
	if !strings.Contains(f.Calls()[0].Input, "открытый вопрос") {
		t.Error("модели не сказано, на какой акт она отвечает")
	}
}
