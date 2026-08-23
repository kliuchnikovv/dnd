package actor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
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
	if !strings.Contains(f.Calls()[0].System, "НЕ ВЫДУМЫВАЕШЬ") {
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

// --- проверка на выдумку ---

type stubGuard struct {
	ok       bool
	err      error
	material []string
	what     string
	// verdicts — вердикты по вызовам: ремонт проверяется второй раз.
	verdicts []bool
	lines    []string
	calls    int
}

func (g *stubGuard) Check(_ context.Context, line string, material []string, _ llm.Request) (Verdict, error) {
	g.material = material
	g.lines = append(g.lines, line)
	ok := g.ok
	if len(g.verdicts) > 0 {
		ok = g.verdicts[min(g.calls, len(g.verdicts)-1)]
	}
	g.calls++
	return Verdict{OK: ok, What: g.what}, g.err
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
	if !strings.Contains(prompt, "Про дело ты можешь утверждать ТОЛЬКО это") {
		t.Errorf("факты не помечены как ограниченные: %q", prompt)
	}
}

// --- темы: не зачитывать и не повторять ---

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
	if !strings.Contains(sys, "Отвечай на сказанное") {
		t.Error("в промпте нет требования отвечать на сказанное")
	}
	if !strings.Contains(f.Calls()[0].Input, "своими словами") {
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

func containsMove(all []string, m string) bool {
	for _, a := range all {
		if a == m {
			return true
		}
	}
	return false
}

// «До встречи» — прощание. Промах словаря сам по себе не страшен, но здесь он
// открывает deflect там, где игрок просто уходит.
func TestFarewellRecognisesCommonForms(t *testing.T) {
	for _, s := range []string{"Жаль, тогда до встречи", "ну бывай", "до встречи"} {
		if got := Classify(s); got != ActFarewell {
			t.Errorf("%q классифицировано как %s, ожидалось farewell", s, got)
		}
	}
}

// Приветствие и прощание мир не меняют. Платить за них как за ход, который
// его меняет, — прямая утечка бюджета на off-case репликах.
func TestFlavourActsGoCheapAndShort(t *testing.T) {
	cases := map[string]llm.Tier{
		"Здравствуйте":                llm.TierCheap,
		"Спасибо":                     llm.TierCheap,
		"До встречи":                  llm.TierCheap,
		"Что-нибудь слышно в городе?": llm.TierMain,
		"Почему вы этого не сказали?": llm.TierMain,
	}
	for text, want := range cases {
		if got := tierFor(Classify(text)); got != want {
			t.Errorf("%q: тир %q, ожидался %q", text, got, want)
		}
	}
	if maxTokensFor(ActGreeting) >= maxTokensFor(ActProbe) {
		t.Error("у приветствия потолок вывода не короче, чем у открытого вопроса")
	}
}

// Проверка на выдумку — это «да/нет». Она не сочиняет и дорогой модели не
// требует никогда.
// Проверка идёт основным тиром, и это не расточительность, а измерение:
// дешёвый судья пропускает одну утечку из десяти (guard_corpus_test.go), а
// утечка бьёт в решаемость дела.
func TestGuardGoesMainTier(t *testing.T) {
	if got := NewGuard(nil).tier(); got != llm.TierMain {
		t.Errorf("страж пошёл тиром %q — дешёвый пропускает утечки", got)
	}
}

// Желание — то, чем персонаж отличается от справочника. Реактивный NPC только
// отвечает; персонаж с желанием сам открывает разговор и просит.
func TestWantOpensAMoveOfItsOwn(t *testing.T) {
	sit := Situation{PlayerText: "Здравствуйте", Wants: []string{
		"передать Ивару, что смена не придёт",
	}}
	allowed := movesFor(Classify(sit.PlayerText), sit)
	if !containsMove(allowed, string(MoveRaiseWant)) {
		t.Errorf("персонажу с желанием нечем его высказать: %v", allowed)
	}

	without := Situation{PlayerText: "Здравствуйте"}
	if containsMove(movesFor(Classify(without.PlayerText), without), string(MoveRaiseWant)) {
		t.Error("ход появился у персонажа без желания")
	}
}

// Желание — разрешённый материал: иначе страж зарубит собственную просьбу
// персонажа как выдумку.
func TestWantIsAllowedMaterial(t *testing.T) {
	sit := Situation{Wants: []string{"передать Ивару, что смена не придёт"}}
	material := allowedMaterial(Speaker{Name: "Берн"}, sit, "")
	for _, m := range material {
		if m == "передать Ивару, что смена не придёт" {
			return
		}
	}
	t.Errorf("желание не попало в материал: %v", material)
}

// Дно для желания — само желание: просьба, сказанная плоско, всё равно
// остаётся просьбой и всё равно двигает разговор.
func TestWantTemplateAsksForTheThing(t *testing.T) {
	sit := Situation{Wants: []string{"передать Ивару, что смена не придёт"}}
	if got := template(MoveRaiseWant, sit); got != sit.Wants[0] {
		t.Errorf("заглушка желания: %q", got)
	}
}

// Желание должно доехать до модели: ход есть, материал разрешён, но если
// текста желания нет в промпте, персонаж попросит наугад.
func TestPromptCarriesTheWant(t *testing.T) {
	sit := Situation{
		PlayerText: "Здравствуйте",
		Wants:      []string{"передать Ивару, что смена не придёт"},
	}
	got := renderPrompt(Speaker{Name: "Берн", Voice: "сухой"}, sit, Classify(sit.PlayerText))
	if !strings.Contains(got, "передать Ивару") {
		t.Errorf("желания нет в промпте:\n%s", got)
	}
}

// --- память разговора ---

// Однокадровый актёр не помнит, что игрок представился ходом раньше. История
// уходит в промпт, иначе «держать контекст» для него невозможно в принципе.
func TestPromptCarriesConversationHistory(t *testing.T) {
	a, f := actorWith(t, `{"move":"deflect","line":"Ничего."}`)
	_, err := a.Line(context.Background(), Speaker{Name: "Берн", Voice: "сухой"},
		Situation{Verb: "talk_to", PlayerText: "а про лодку?",
			History: []store.Exchange{
				{Player: "здравствуйте", Reply: "и вам не хворать", Turn: 1},
			}}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	in := f.Calls()[0].Input
	if !strings.Contains(in, "здравствуйте") || !strings.Contains(in, "и вам не хворать") {
		t.Errorf("разговор не доехал в промпт:\n%s", in)
	}
	// Прошлое обязано стоять до текущей реплики: промпт читается сверху вниз.
	if strings.Index(in, "здравствуйте") > strings.Index(in, "а про лодку?") {
		t.Errorf("прошлое встало после текущей реплики:\n%s", in)
	}
}

// Сказанное запоминается на месте: иначе следующий ход снова начнёт с нуля.
func TestVoiceRemembersExchange(t *testing.T) {
	g := harbour(t)
	v, _ := voicer(t, g, `{"move":"smalltalk","line":"Мокро сегодня."}`)
	v.Turn = func() int { return 7 }

	in := core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_bern", Text: "здравствуйте"}}
	if _, err := v.Voice(context.Background(), in, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}

	got := g.D.Recent("e_bern")
	if len(got) != 1 {
		t.Fatalf("запомнилось %d кругов, ждали 1", len(got))
	}
	if got[0].Player != "здравствуйте" || got[0].Reply != "Мокро сегодня." || got[0].Turn != 7 {
		t.Errorf("круг записан неверно: %+v", got[0])
	}
}

// Второй ход видит первый: память замкнута, а не только пишется.
func TestVoiceFeedsRememberedBack(t *testing.T) {
	g := harbour(t)
	v, f := voicer(t, g, `{"move":"smalltalk","line":"Мокро сегодня."}`)
	in := core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_bern", Text: "здравствуйте"}}
	if _, err := v.Voice(context.Background(), in, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Voice(context.Background(), in, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if got := f.Calls()[1].Input; !strings.Contains(got, "Мокро сегодня.") {
		t.Errorf("второй ход не увидел первого:\n%s", got)
	}
}

// Молчание не запоминается: пустой круг в транскрипте — это шум, который
// вытесняет сказанное.
func TestVoiceDoesNotRememberSilence(t *testing.T) {
	g := harbour(t)
	v, _ := voicer(t, g, `{"move":"deflect","line":"тишина"}`)
	in := core.Intent{Verb: "examine", Args: core.Args{Target: "e_body"}}
	if _, err := v.Voice(context.Background(), in, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if got := g.D.Recent("e_bern"); len(got) != 0 {
		t.Errorf("молчание попало в память: %+v", got)
	}
}

// --- диалоговый актёр: реплика без закрытого набора ходов ---

// Свободная реплика доходит до игрока как есть. Набор ходов больше не
// грамматика: ограничивать надо не то, КАК персонаж говорит, а то, ЧТО он
// вправе утверждать о деле.
func TestFreeLinePassesThrough(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, _ := actorWith(t, `{"line":"И вам не хворать. Сыро, да."}`)
	got, err := a.Line(context.Background(), sp, Situation{
		Verb: "talk_to", PlayerText: "здравствуйте",
		Known: KnownTopics(g), Scene: SceneOf(g, "e_bern"),
	}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "И вам не хворать. Сыро, да." {
		t.Errorf("реплика %q — свободная речь не прошла", got)
	}
}

// Чего персонаж не знает, он запрашивает, а не сочиняет. Мастера ещё нет,
// поэтому запрос пока только разбирается.
func TestNeedsAndSecretHintAreParsed(t *testing.T) {
	a, _ := actorWith(t, `{"line":"Спросите в конторе.","needs":["кто держит ключи от весовой"],`+
		`"hinting_secret":true}`)
	out, err := a.speak(context.Background(), Speaker{Name: "Берн", Voice: "сухой"},
		Situation{Verb: "talk_to"}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Line != "Спросите в конторе." {
		t.Errorf("реплика %q", out.Line)
	}
	if len(out.Needs) != 1 || out.Needs[0] != "кто держит ключи от весовой" {
		t.Errorf("запрос к Мастеру не разобрался: %v", out.Needs)
	}
	if !out.HintingSecret {
		t.Error("намёк на секрет не разобрался")
	}
}

// Схема — это то, что модель вообще может сказать. Закрытого набора ходов в
// ней больше нет; запрос к Мастеру есть.
func TestSchemaIsDialogueShaped(t *testing.T) {
	g := harbour(t)
	props := schemaFor(topicsOf(g, "e_bern"))["properties"].(map[string]any)
	if _, ok := props["move"]; ok {
		t.Error("в схеме остался закрытый набор ходов")
	}
	if _, ok := props["fact"]; ok {
		t.Error("в схеме осталось подтверждение факта по идентификатору")
	}
	for _, want := range []string{"line", "needs", "hinting_secret"} {
		if _, ok := props[want]; !ok {
			t.Errorf("в схеме нет поля %q", want)
		}
	}
}

// Персонаж говорит от себя, а не «озвучивает NPC»: из роли не выходит.
func TestSystemPromptSpeaksAsThePerson(t *testing.T) {
	a, f := actorWith(t, `{"line":"Сыро."}`)
	if _, err := a.Line(context.Background(), Speaker{Name: "Берн", Voice: "сухой"},
		Situation{Verb: "talk_to"}, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	sys := f.Calls()[0].System
	if !strings.Contains(sys, "Ты —") {
		t.Errorf("промпт не от лица человека:\n%s", sys)
	}
	if strings.Contains(sys, "озвучиваешь") {
		t.Errorf("промпт всё ещё про озвучку роли:\n%s", sys)
	}
	// Три источника знания и запрос вместо выдумки — несущая часть промпта.
	if !strings.Contains(sys, "needs") {
		t.Errorf("промпт не говорит, как запросить у Мастера:\n%s", sys)
	}
}

// Затравка мира — рамка, за которую можно цепляться, не выдумывая. Без неё
// персонажу нечего сказать о себе и о посёлке.
func TestSettingAndLifeReachThePrompt(t *testing.T) {
	g := harbour(t)
	v, f := voicer(t, g, `{"line":"Сыро."}`)
	if _, err := v.Voice(context.Background(),
		core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_bern"}},
		core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	in := f.Calls()[0].Input
	if !strings.Contains(in, "посёлок в устье") {
		t.Errorf("сеттинг не доехал в промпт:\n%s", in)
	}
	if !strings.Contains(in, "караулке") {
		t.Errorf("быт персонажа не доехал в промпт:\n%s", in)
	}
}

// Пустая реплика — не молчание, а сбой: игрок обязан получить фразу.
func TestEmptyLineFallsBackToTemplate(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, _ := actorWith(t, `{"line":""}`)
	got, err := a.Line(context.Background(), sp,
		Situation{Verb: "talk_to", Scene: SceneOf(g, "e_bern")}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Error("шаблон не подставился")
	}
}

// Тема остаётся структурным хинтом: названная — помечается рассказанной,
// чтобы не звучать второй раз. Но названная неверно реплику не рубит:
// грамматикой набор больше не является.
func TestUnknownTopicDoesNotKillTheLine(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, _ := actorWith(t, `{"line":"Книгу через пристань таскают, я докладывал.","topic":"t99"}`)
	got, err := a.Line(context.Background(), sp, Situation{
		Verb: "talk_to", PlayerText: "что слышно?", Talks: topicsOf(g, "e_bern"),
	}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Книгу через пристань таскают, я докладывал." {
		t.Errorf("реплика срезана из-за хинта: %q", got)
	}
}

// Отсутствующий ключ флейвора отдаёт «[ключ]». В промпте это дыра, а не
// рамка: персонаж начинает объяснять модели самому себе, что описано пусто.
func TestMissingFrameDoesNotReachThePrompt(t *testing.T) {
	g := harbour(t)
	v, f := voicer(t, g, `{"line":"Сыро."}`)
	if _, err := v.Voice(context.Background(),
		core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_bern"}},
		core.TurnResult{FlavourKey: "нет.такого.ключа"}); err != nil {
		t.Fatal(err)
	}
	if in := f.Calls()[0].Input; strings.Contains(in, "[нет.такого.ключа]") ||
		strings.Contains(in, "Что уже описано: []") {
		t.Errorf("дыра флейвора доехала в промпт:\n%s", in)
	}
}

// --- протокол: актёр запрашивает данные у Мастера ---

// fakeMaster — Мастер под контролем теста. Реальный живёт в пакете master и
// проверяется там; здесь проверяется протокол, а не его решения.
type fakeMaster struct {
	grants  []master.Grant
	answers []string // ответы по вызовам: Мастер вправе надумать новое
	refuse  []string
	err     error
	calls   int
	needs   []string
	canon   []master.CanonFact
	setting string
}

func (m *fakeMaster) Grant(_ context.Context, needs []string, canon []master.CanonFact,
	w master.World, _ llm.Request) ([]master.Grant, []string, error) {
	m.needs = append(m.needs, needs...)
	m.canon = canon
	m.setting = w.Setting
	if len(m.answers) > 0 {
		answer := m.answers[min(m.calls, len(m.answers)-1)]
		m.calls++
		return []master.Grant{{Topic: "ключи от весовой", Answer: answer, Canon: true}},
			nil, m.err
	}
	m.calls++
	return m.grants, m.refuse, m.err
}

// Реплики по кругу: первая с запросом, вторая — договаривает.
func repliesInOrder(t *testing.T, replies ...string) (*Actor, *llm.Fake) {
	t.Helper()
	var i int
	f := llm.NewFake("fake", true).ReplyWith(func(llm.Request) string {
		out := replies[min(i, len(replies)-1)]
		i++
		return out
	})
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleActor, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	return New(gw), f
}

// Круг ровно один: актёр спросил, Мастер ответил, актёр договорил.
func TestNeedsGoToMasterAndBack(t *testing.T) {
	g := harbour(t)
	a, f := repliesInOrder(t,
		`{"line":"Сейчас скажу.","needs":["кто держит ключи от весовой"]}`,
		`{"line":"Ключи у смотрителя весов, к нему и идите."}`)
	m := &fakeMaster{grants: []master.Grant{
		{Topic: "ключи от весовой", Answer: "у смотрителя весов", Canon: true}}}
	v := &GameVoicer{Actor: a, Game: g, Master: m, Turn: func() int { return 2 }}

	got, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "а ключи у кого?"}}, core.TurnResult{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Ключи у смотрителя весов, к нему и идите." {
		t.Errorf("реплика %q — актёр не договорил с данными Мастера", got)
	}
	if m.calls != 1 {
		t.Errorf("Мастер вызван %d раз, круг должен быть один", m.calls)
	}
	if len(f.Calls()) != 2 {
		t.Errorf("вызовов актёра %d, ждали два", len(f.Calls()))
	}
	if in := f.Calls()[1].Input; !strings.Contains(in, "у смотрителя весов") {
		t.Errorf("ответ Мастера не доехал во второй вызов:\n%s", in)
	}
	if m.setting == "" {
		t.Error("Мастер решает без сеттинга дела")
	}
}

// Решённое Мастером оседает в каноне и держит слово: спросят второй раз —
// вернётся то же, даже если Мастер надумал новое. Без этого «расширение мира»
// становится генератором противоречий.
func TestGrantedDetailBecomesCanonAndHoldsIt(t *testing.T) {
	g := harbour(t)
	a, f := repliesInOrder(t,
		`{"line":"Сейчас.","needs":["кто держит ключи от весовой"]}`,
		`{"line":"У смотрителя весов."}`,
		`{"line":"Сейчас.","needs":["кто держит ключи от весовой"]}`,
		`{"line":"Я же сказал — у смотрителя."}`)
	m := &fakeMaster{answers: []string{"у смотрителя весов", "у начальника стражи"}}
	v := &GameVoicer{Actor: a, Game: g, Master: m}
	in := core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_bern", Text: "ключи?"}}

	if _, err := v.Voice(context.Background(), in, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if got, ok := g.CanonGet("ключи от весовой"); !ok || got != "у смотрителя весов" {
		t.Fatalf("деталь не осела в каноне: %q (%v)", got, ok)
	}
	if _, err := v.Voice(context.Background(), in, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := g.CanonGet("ключи от весовой"); got != "у смотрителя весов" {
		t.Errorf("канон переписан вторым ответом Мастера: %q", got)
	}
	// Актёр договаривает действующим каноном, а не свежей выдумкой Мастера.
	last := f.Calls()[len(f.Calls())-1].Input
	if !strings.Contains(last, "у смотрителя весов") || strings.Contains(last, "начальника стражи") {
		t.Errorf("актёру доехал не канон:\n%s", last)
	}
}

// Мимолётное каноном не становится: настроение и «кто сейчас прошёл мимо» в
// канон писать нельзя, иначе он зарастёт шумом.
func TestNonCanonGrantIsNotRemembered(t *testing.T) {
	g := harbour(t)
	a, _ := repliesInOrder(t,
		`{"line":"Сейчас.","needs":["кто там шумит"]}`, `{"line":"Грузчики бранятся."}`)
	m := &fakeMaster{grants: []master.Grant{
		{Topic: "кто там шумит", Answer: "грузчики бранятся у весов", Canon: false}}}
	v := &GameVoicer{Actor: a, Game: g, Master: m}

	if _, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "что за шум?"}}, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if len(g.Canon()) != 0 {
		t.Errorf("мимолётное ушло в канон: %+v", g.Canon())
	}
}

// Отказ Мастера — законный исход: персонаж договаривает честным уклонением,
// а не выдумкой и не заглушкой.
func TestMasterRefusalReachesTheActor(t *testing.T) {
	g := harbour(t)
	a, f := repliesInOrder(t,
		`{"line":"Сейчас.","needs":["кто убил Халдена"]}`,
		`{"line":"Про такое я не знаю, и знать не хочу."}`)
	m := &fakeMaster{refuse: []string{"кто убил Халдена"}}
	v := &GameVoicer{Actor: a, Game: g, Master: m}

	got, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "кто убил?"}}, core.TurnResult{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Про такое я не знаю, и знать не хочу." {
		t.Errorf("реплика %q", got)
	}
	if in := f.Calls()[1].Input; !strings.Contains(in, "кто убил Халдена") {
		t.Errorf("отказ не доехал до актёра:\n%s", in)
	}
}

// Второй круг запрещён: needs второго прохода игнорируются, иначе разговор
// уходит в цикл запросов и съедает потолок вызовов на ход.
func TestSecondRoundNeedsAreIgnored(t *testing.T) {
	g := harbour(t)
	a, f := repliesInOrder(t,
		`{"line":"Сейчас.","needs":["кто держит ключи"]}`,
		`{"line":"Спросите в конторе.","needs":["а кто в конторе сидит"]}`)
	m := &fakeMaster{grants: []master.Grant{
		{Topic: "ключи", Answer: "у смотрителя", Canon: true}}}
	v := &GameVoicer{Actor: a, Game: g, Master: m}

	got, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "ключи?"}}, core.TurnResult{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Спросите в конторе." {
		t.Errorf("реплика %q", got)
	}
	if m.calls != 1 || len(f.Calls()) != 2 {
		t.Errorf("круг не один: Мастер %d, актёр %d", m.calls, len(f.Calls()))
	}
	if in := f.Calls()[1].Input; !strings.Contains(in, "больше не спрашивай") {
		t.Errorf("актёру не сказано, что круг закончен:\n%s", in)
	}
}

// Сбой Мастера не рушит ход: персонаж договаривает тем, что у него есть.
func TestMasterFailureLeavesTheFirstLine(t *testing.T) {
	g := harbour(t)
	a, f := repliesInOrder(t, `{"line":"Не знаю, право слово.","needs":["кто держит ключи"]}`)
	m := &fakeMaster{err: errors.New("шлюз закрыт")}
	v := &GameVoicer{Actor: a, Game: g, Master: m}

	got, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "ключи?"}}, core.TurnResult{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Не знаю, право слово." {
		t.Errorf("реплика %q — сбой Мастера съел ход", got)
	}
	if len(f.Calls()) != 1 {
		t.Error("после сбоя Мастера актёра переспросили впустую")
	}
}

// Без Мастера запрос просто игнорируется: игра без него работает как раньше.
func TestNeedsWithoutMasterAreIgnored(t *testing.T) {
	g := harbour(t)
	a, f := repliesInOrder(t, `{"line":"Кто их знает.","needs":["кто держит ключи"]}`)
	v := &GameVoicer{Actor: a, Game: g}

	got, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "ключи?"}}, core.TurnResult{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Кто их знает." || len(f.Calls()) != 1 {
		t.Errorf("реплика %q, вызовов %d", got, len(f.Calls()))
	}
}

// Канон — материал персонажа: решённую деталь он вправе озвучивать сам, не
// запрашивая её второй раз.
func TestCanonReachesThePrompt(t *testing.T) {
	g := harbour(t)
	g.CanonPut("ключи от весовой", "у смотрителя весов", 1)
	v, f := voicer(t, g, `{"line":"У смотрителя."}`)

	if _, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "ключи?"}}, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if in := f.Calls()[0].Input; !strings.Contains(in, "у смотрителя весов") {
		t.Errorf("канон не доехал в промпт:\n%s", in)
	}
}

// --- ремонт вместо заглушки ---

// Сработавшая проверка не обязана стоить игроку голоса персонажа: сначала
// переспрос «то же, без выдуманного», и только потом дно.
func TestGuardRejectionAsksForRepairFirst(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, f := repliesInOrder(t,
		`{"line":"Дежурства удвоили с прошлой недели, спросите Олсена."}`,
		`{"line":"Дежурства как дежурства. Сыро только."}`)
	sg := &stubGuard{verdicts: []bool{false, true}, what: "фермер Олсен"}
	a = a.WithGuard(sg)

	got, err := a.Line(context.Background(), sp,
		Situation{Verb: "talk_to", Scene: SceneOf(g, "e_bern")}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Дежурства как дежурства. Сыро только." {
		t.Errorf("реплика %q — ремонт не подставился", got)
	}
	if len(f.Calls()) != 2 {
		t.Fatalf("вызовов модели %d, ждали два: реплика и ремонт", len(f.Calls()))
	}
	repair := f.Calls()[1]
	if !strings.Contains(repair.Input, "фермер Олсен") {
		t.Errorf("ремонту не сказано, что именно убрать:\n%s", repair.Input)
	}
	if !strings.Contains(repair.Input, "Дежурства удвоили") {
		t.Errorf("ремонту не дана та же реплика:\n%s", repair.Input)
	}
	if repair.Tier != llm.TierCheap {
		t.Errorf("ремонт пошёл тиром %q — за него платится на каждой утечке", repair.Tier)
	}
	// Отремонтированное проверяется снова: ремонт вправе подставить вторую
	// выдумку вместо первой.
	if sg.calls != 2 {
		t.Errorf("проверок %d — отремонтированная реплика не проверена", sg.calls)
	}
}

// Провал ремонта — вот единственное место для заглушки.
func TestFloorOnlyAfterRepairFails(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, _ := repliesInOrder(t,
		`{"line":"Спросите Олсена."}`, `{"line":"Спросите всё равно Олсена."}`)
	a = a.WithGuard(&stubGuard{ok: false, what: "Олсен"})

	got, err := a.Line(context.Background(), sp,
		Situation{Verb: "talk_to", Scene: SceneOf(g, "e_bern")}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Олсен") {
		t.Errorf("выдумка дошла до игрока: %q", got)
	}
	if got == "" {
		t.Error("дно не подставилось")
	}
}

// Сбой канала на ремонте не рушит ход: игрок получает дно, а не ошибку.
func TestRepairChannelFailureFallsToFloor(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	f := llm.NewFake("fake", true).ReplyWith(func(r llm.Request) string {
		if strings.Contains(r.System, "то же самое") {
			return "не json"
		}
		return `{"line":"Спросите Олсена."}`
	})
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleActor, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	a := New(gw).WithGuard(&stubGuard{ok: false, what: "Олсен"})

	got, err := a.Line(context.Background(), sp, Situation{Verb: "talk_to"}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got == "" || strings.Contains(got, "Олсен") {
		t.Errorf("реплика %q", got)
	}
}

// --- дно: бледно, но по-человечески ---

// Дно — последняя фраза, которую слышит игрок вместо персонажа. Канцелярская
// формулировка здесь читается как сбой движка, а не как характер.
func TestFloorPhrasesAreHuman(t *testing.T) {
	sit := Situation{Scene: []string{"Место: Пристань", "Обстановка: дождь"}}
	seen := map[string]bool{}
	for _, m := range moves() {
		got := template(Move(m), sit)
		if strings.TrimSpace(got) == "" {
			t.Errorf("ход %q не имеет дна", m)
		}
		if got == "Тут я вам не помогу." {
			t.Errorf("ход %q провалился в канцелярскую заглушку", m)
		}
		seen[got] = true
	}
	// Уклонение — самый частый ход, и у него обязана быть своя фраза, а не
	// общий дефолт: именно её игрок слышит чаще всего.
	if template(MoveDeflect, sit) == template(Move("нет такого хода"), sit) {
		t.Error("у уклонения нет своей фразы — оно падает в общий дефолт")
	}
	if len(seen) < 5 {
		t.Errorf("дно однообразно: %d разных фраз на %d ходов", len(seen), len(moves()))
	}
}

// Сбой Мастера не рушит ход, но и молчать о нём нельзя: незароученная роль
// выглядит как «модель ответила бледно», и так она живёт неделями.
func TestMasterFailureIsReported(t *testing.T) {
	g := harbour(t)
	a, _ := repliesInOrder(t, `{"line":"Кто ж его знает.","needs":["кто держит ключи"]}`)
	var noted []string
	v := &GameVoicer{Actor: a, Game: g,
		Master: &fakeMaster{err: errors.New("роль не зароутена")},
		Notify: func(err error) { noted = append(noted, err.Error()) }}

	if _, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "ключи?"}}, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if len(noted) != 1 || !strings.Contains(noted[0], "роль не зароутена") {
		t.Errorf("о сбое Мастера не сообщено: %v", noted)
	}
}

// Пустой ответ — почти всегда упёртый потолок вывода: модель с рассуждением
// тратит его до того, как напишет реплику. Живой прогон отдал ровно
// MaxTokens и пустую строку, а игрок увидел «реплика не разобралась».
func TestEmptyResponseNamesTheOutputCap(t *testing.T) {
	a, _ := actorWith(t, "")
	_, err := a.Line(context.Background(), Speaker{Name: "Берн", Voice: "сухой"},
		Situation{Verb: "talk_to"}, llm.Request{})
	if err == nil {
		t.Fatal("пустой ответ прошёл как реплика")
	}
	if !strings.Contains(err.Error(), "потолок вывода") {
		t.Errorf("причина не названа: %v", err)
	}
}

// Потолок вывода считается не по длине реплики: реплика короткая, но модель
// с рассуждением тратит вывод раньше, чем доходит до неё.
func TestOutputCapLeavesRoomForReasoning(t *testing.T) {
	if got := maxTokensFor(ActProbe); got < 600 {
		t.Errorf("потолок основного тира %d — рассуждению не хватит", got)
	}
	if got := maxTokensFor(ActGreeting); got < 200 {
		t.Errorf("потолок дешёвого тира %d — рассуждению не хватит", got)
	}
}

// --- реплика это только слова вслух ---

// Оформление ставит код: реплика NPC и реплика игрока обязаны выглядеть
// одинаково. Модель, начавшая со своего тире, даёт «— — Здравствуйте».
func TestCleanStripsLeadingDash(t *testing.T) {
	for _, in := range []string{"— Здравствуйте.", "- Здравствуйте.", "—Здравствуйте.",
		"«— Здравствуйте.»"} {
		if got := clean(in); got != "Здравствуйте." {
			t.Errorf("clean(%q) = %q", in, got)
		}
	}
}

// Ремарка в реплике — чужая работа: описывает мир Мастер, персонаж только
// говорит. «— Здравствуйте. — Отряхиваю воду с плаща.» это две разные роли в
// одной строке.
func TestPromptsForbidStageDirections(t *testing.T) {
	a, f := actorWith(t, `{"line":"Здравствуйте."}`)
	if _, err := a.Line(context.Background(), Speaker{Name: "Берн", Voice: "сухой"},
		Situation{Verb: "talk_to"}, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	if sys := f.Calls()[0].System; !strings.Contains(sys, "ремарок") {
		t.Errorf("промпт не запрещает ремарки:\n%s", sys)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(f.Calls()[0].Schema), &probe); err != nil {
		t.Fatal(err)
	}
	line := probe["properties"].(map[string]any)["line"].(map[string]any)
	if desc, _ := line["description"].(string); !strings.Contains(desc, "вслух") {
		t.Errorf("в схеме не сказано, что line — только произносимое: %q", desc)
	}
}

// Ремонт тоже пишет реплику, и правила у неё те же.
func TestRepairPromptForbidsStageDirections(t *testing.T) {
	g := harbour(t)
	sp, _ := SpeakerFor(g, "e_bern")
	a, f := repliesInOrder(t, `{"line":"Спросите Олсена."}`, `{"line":"Кто ж его знает."}`)
	a = a.WithGuard(&stubGuard{verdicts: []bool{false, true}, what: "Олсен"})
	if _, err := a.Line(context.Background(), sp, Situation{Verb: "talk_to"}, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	if sys := f.Calls()[1].System; !strings.Contains(sys, "ремарок") {
		t.Errorf("ремонту разрешены ремарки:\n%s", sys)
	}
}

// Мастер, потянувшийся к территории дела, отсекается ядром — и персонаж об
// этом не говорит. Иначе ambient-канон стал бы вторым способом выдать факт,
// мимо fact_holders: гейт держит границу там, где промпт уже не держит.
func TestMasterAnswerOnCaseFactIsNotVoiced(t *testing.T) {
	g := harbour(t)
	a, f := repliesInOrder(t,
		`{"line":"Сейчас.","needs":["кто соврал про ночь"]}`,
		`{"line":"Про это я говорить не стану."}`)
	m := &fakeMaster{grants: []master.Grant{
		{Topic: "f_toke_lied", Answer: "Токе соврал", Canon: true}}}
	v := &GameVoicer{Actor: a, Game: g, Master: m}

	if _, err := v.Voice(context.Background(), core.Intent{Verb: "talk_to",
		Args: core.Args{Target: "e_bern", Text: "кто соврал?"}}, core.TurnResult{}); err != nil {
		t.Fatal(err)
	}
	if got := g.Canon(); len(got) != 0 {
		t.Errorf("факт дела уехал в канон: %+v", got)
	}
	if in := f.Calls()[1].Input; strings.Contains(in, "Токе соврал") {
		t.Errorf("отвергнутая деталь доехала до актёра как материал:\n%s", in)
	}
}
