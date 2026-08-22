package intent

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
	"github.com/kliuchnikovv/dnd/store"
)

// parserWith собирает парсер, чей провайдер отвечает заданным JSON.
func parserWith(t *testing.T, replyJSON string) (*Parser, *llm.Fake) {
	t.Helper()
	f := llm.NewFake("fake", true).ReplyWith(func(llm.Request) string { return replyJSON })
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleIntentParser, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	return NewParser(gw), f
}

func harbourGame(t *testing.T) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	return core.NewGame(*cfg)
}

func harbourHint(t *testing.T) SceneHint {
	t.Helper()
	return BuildHint(harbourGame(t))
}

func parse(t *testing.T, replyJSON string, hint SceneHint) Result {
	t.Helper()
	p, _ := parserWith(t, replyJSON)
	got, err := p.Parse(context.Background(), "любой ввод", hint, llm.Request{})
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	return got
}

// Схема отдаёт модели все глаголы, исполнимые в этой сцене. «Все» значит все
// из реестра, когда сцена может дать каждому обязательный аргумент — а
// предметные глаголы берут свой из разных мест: инструмент из узла, предъявление
// из носимого.
func TestSchemaEnumeratesEveryVerb(t *testing.T) {
	s := SchemaFor(SceneHint{
		Tools:   []Named{{"p_hammers", "Молоты"}},
		Carried: []Named{{"i_writ", "Предписание магистрата"}},
	})
	props := s["properties"].(map[string]any)
	verb := props["verb"].(map[string]any)
	enum := verb["enum"].([]string)
	if len(enum) != len(core.Verbs) {
		t.Errorf("в схеме %d глаголов, в реестре %d", len(enum), len(core.Verbs))
	}
	if s["additionalProperties"] != false {
		t.Error("additionalProperties не запрещены — strict-режим без этого не гарантия")
	}
}

// Схема обязана быть побайтово стабильной: у провайдеров грамматики
// кэшируются, и плавающий порядок ключей этот кэш обнуляет.
func TestSchemaJSONIsStable(t *testing.T) {
	if SchemaJSON() != SchemaJSON() {
		t.Fatal("схема нестабильна между вызовами")
	}
}

func TestParsesQuestionWithKnownTopic(t *testing.T) {
	hint := harbourHint(t)
	if len(hint.Topics) == 0 {
		t.Fatal("в деле нет стартовых тем — фикстура сломана")
	}
	topic := hint.Topics[0].ID
	target := hint.Entities[0].ID

	got := parse(t, `{"outcome":"intent","verb":"question","target":"`+target+`","topic":"`+topic+`"}`, hint)
	if !got.Accepted() {
		t.Fatalf("не принято: clarify=%q candidate=%q", got.Clarify, got.Candidate)
	}
	if got.Intent.Verb != "question" {
		t.Errorf("глагол %q", got.Intent.Verb)
	}
	if string(got.Intent.Args.Topic) != topic {
		t.Errorf("тема %q, ожидалась %q", got.Intent.Args.Topic, topic)
	}
	if got.Class != core.ClassInvestigate {
		t.Errorf("класс %q", got.Class)
	}
}

// Главная проверка: схема отвечает за форму, код — за смысл. Ссылка на
// сущность вне сцены обязана быть отклонена, даже будучи формально валидной.
func TestRejectsTargetOutsideScene(t *testing.T) {
	hint := harbourHint(t)
	got := parse(t, `{"outcome":"intent","verb":"examine","target":"e_призрак"}`, hint)
	if got.Accepted() {
		t.Fatal("принята ссылка на сущность, которой нет в сцене")
	}
	if got.Clarify == "" {
		t.Error("отказ без вопроса игроку — это и есть клетка закрытого словаря")
	}
	if got.Class != core.ClassInvestigate {
		t.Errorf("класс отказа %q, ожидался investigate", got.Class)
	}
}

// Защита от угадывания держится и на входе: спросить о факте, которого парти
// не знает, нельзя, даже если модель это предложила.
func TestRejectsUnknownTopicEvenWhenModelProposesIt(t *testing.T) {
	hint := harbourHint(t)
	got := parse(t, `{"outcome":"intent","verb":"question","target":"`+hint.Entities[0].ID+
		`","topic":"f_правда_о_убийце"}`, hint)
	if got.Accepted() {
		t.Fatal("пропущена тема вне банка знаний — защита от угадывания дырява")
	}
	if !strings.Contains(got.Clarify, "не знает") {
		t.Errorf("сообщение об отказе: %q", got.Clarify)
	}
}

func TestRejectsUnreachableNode(t *testing.T) {
	hint := harbourHint(t)
	got := parse(t, `{"outcome":"intent","verb":"move_zone","node":"n_луна"}`, hint)
	if got.Accepted() {
		t.Fatal("принят недостижимый узел")
	}
}

func TestRejectsVerbOutsideRegistry(t *testing.T) {
	got := parse(t, `{"outcome":"intent","verb":"телепортироваться"}`, harbourHint(t))
	if got.Accepted() {
		t.Fatal("принят глагол вне реестра")
	}
	if got.Class != "" {
		t.Errorf("промах по схеме приписан классу %q", got.Class)
	}
}

func TestClarifyAndUnsupportedAreNotErrors(t *testing.T) {
	hint := harbourHint(t)

	c := parse(t, `{"outcome":"clarify","clarify":"К кузнецу или к стражнику?"}`, hint)
	if c.Accepted() || c.Clarify != "К кузнецу или к стражнику?" {
		t.Errorf("clarify разобран неверно: %+v", c)
	}

	u := parse(t, `{"outcome":"unsupported","reason":"нет глагола для подкупа"}`, hint)
	if u.Accepted() || u.Candidate != "нет глагола для подкупа" {
		t.Errorf("unsupported разобран неверно: %+v", u)
	}
}

// Парсер обязан договариваться, а не молчать: пустой clarify от модели
// заменяется осмысленным вопросом.
func TestEmptyClarifyGetsFallbackQuestion(t *testing.T) {
	got := parse(t, `{"outcome":"clarify"}`, harbourHint(t))
	if got.Clarify == "" {
		t.Error("парсер вернул отказ без вопроса")
	}
}

func TestBrokenJSONIsAnError(t *testing.T) {
	p, _ := parserWith(t, `{не json`)
	if _, err := p.Parse(context.Background(), "x", harbourHint(t), llm.Request{}); err == nil {
		t.Fatal("битый ответ не дал ошибки")
	}
}

// Подсказка не имеет права нести ничего сверх того, что игрок уже видит.
func TestHintLeaksNeitherTruthNorUnknownFacts(t *testing.T) {
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	g := core.NewGame(*cfg)
	hint := BuildHint(g)
	rendered := hint.Render()

	known := map[string]bool{}
	for _, f := range g.K.TopicBank() {
		known[string(f)] = true
	}
	for _, tp := range hint.Topics {
		if !known[tp.ID] {
			t.Errorf("в подсказке тема %q вне банка знаний парти", tp.ID)
		}
	}
	// Токены правильного ответа «Гавани» не должны встречаться нигде.
	for _, leak := range []string{"seal_cord", "night_before_tide", "audit_shortfall", "redacted"} {
		if strings.Contains(rendered, leak) {
			t.Errorf("подсказка содержит %q", leak)
		}
	}
	if !strings.Contains(rendered, "Известные темы") {
		t.Error("подсказка не перечисляет темы — модель не сможет их выбрать")
	}
}

func TestSchemaIsSentToProviderAsStrict(t *testing.T) {
	hint := harbourHint(t)
	p, f := parserWith(t, `{"outcome":"clarify","clarify":"?"}`)
	if _, err := p.Parse(context.Background(), "что-то", hint, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("вызовов %d", len(calls))
	}
	if calls[0].Schema == "" {
		t.Error("схема не передана провайдеру")
	}
	if calls[0].Role != llm.RoleIntentParser {
		t.Errorf("роль %q — тиринг и запрет нестрогих провайдеров идут по роли", calls[0].Role)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(calls[0].Schema), &probe); err != nil {
		t.Errorf("переданная схема не JSON: %v", err)
	}
}

// Парсер не может обойти шлюз: провайдер без гарантии схемы к этой роли
// не допускается, и парсер получает ошибку, а не тихий разбор.
func TestParserCannotUseNonStrictProvider(t *testing.T) {
	loose := llm.NewFake("loose", false).ReplyWith(func(llm.Request) string {
		return `{"outcome":"intent","verb":"look"}`
	})
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleIntentParser, llm.Target{Provider: loose, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	p := NewParser(gw)

	if _, err := p.Parse(context.Background(), "смотрю", harbourHint(t), llm.Request{}); err == nil {
		t.Fatal("парсер прошёл через провайдера без гарантии схемы")
	}
	if len(loose.Calls()) != 0 {
		t.Error("нестрогий провайдер был вызван")
	}
}

func TestRejectionRateIsPerVerbClass(t *testing.T) {
	m := NewMetrics()
	// investigate: 1 из 4 отклонён — 25%
	m.Observe(core.ClassInvestigate, true)
	m.Observe(core.ClassInvestigate, true)
	m.Observe(core.ClassInvestigate, true)
	m.Observe(core.ClassInvestigate, false)
	// social: 2 из 2 отклонены — 100%
	m.Observe(core.ClassSocial, false)
	m.Observe(core.ClassSocial, false)

	if r := m.Rate(core.ClassInvestigate); r != 0.25 {
		t.Errorf("investigate %.2f, ожидалось 0.25", r)
	}
	if r := m.Rate(core.ClassSocial); r != 1.0 {
		t.Errorf("social %.2f, ожидалось 1.00", r)
	}
	if r := m.Rate(core.ClassAttack); r != 0 {
		t.Errorf("класс без наблюдений дал %.2f, ожидался 0", r)
	}
	if got := m.Overall(); got != 0.5 {
		t.Errorf("в целом %.2f, ожидалось 0.50", got)
	}
	breaches := m.Breaches(0.25)
	if len(breaches) != 1 || breaches[0] != core.ClassSocial {
		t.Errorf("нарушители порога: %v, ожидался только social", breaches)
	}
}

func TestClasslessRejectionsCountInOverall(t *testing.T) {
	m := NewMetrics()
	m.Observe(core.ClassInvestigate, true)
	m.Observe("", false) // непонятый ввод без глагола
	if got := m.Overall(); got != 0.5 {
		t.Errorf("в целом %.2f, ожидалось 0.50 — игроку всё равно, почему его не поняли", got)
	}
	if r := m.Rate(core.ClassInvestigate); r != 0 {
		t.Errorf("беcклассовый отказ приписан investigate: %.2f", r)
	}
}

func TestObservationsCountsEveryInput(t *testing.T) {
	m := NewMetrics()
	if m.Observations() != 0 {
		t.Fatal("на пустой метрике есть наблюдения")
	}
	m.Observe(core.ClassInvestigate, true)
	m.Observe("", false)
	if got := m.Observations(); got != 2 {
		t.Errorf("наблюдений %d, ожидалось 2", got)
	}
}

// Ровно тот баг, который вылез на живом прогоне: talk_to без цели доходил
// до движка, и ключ флейвора уезжал на узел вместо сущности.
func TestVerbWithoutRequiredTargetBecomesClarify(t *testing.T) {
	hint := harbourHint(t)
	got := parse(t, `{"outcome":"intent","verb":"talk_to"}`, hint)
	if got.Accepted() {
		t.Fatal("talk_to без цели принят как действие")
	}
	if !strings.Contains(got.Clarify, "к кому") {
		t.Errorf("вопрос не про цель: %q", got.Clarify)
	}
	if got.Class != core.ClassSocial {
		t.Errorf("класс %q — отказ должен считаться по классу глагола", got.Class)
	}
}

func TestArityIsCheckedForEveryShape(t *testing.T) {
	hint := harbourHint(t)
	target := hint.Entities[0].ID
	cases := []struct {
		name  string
		reply string
	}{
		{"question без темы", `{"outcome":"intent","verb":"question","target":"` + target + `"}`},
		{"question без цели", `{"outcome":"intent","verb":"question","topic":"x"}`},
		{"move_zone без узла", `{"outcome":"intent","verb":"move_zone"}`},
		// theorize/say/emote в этот список не входят: их обязательный
		// аргумент — свободный текст, и он берётся из фразы игрока, а не
		// выспрашивается у него же (TestFreeTextVerbTakesThePlayersOwnWords).
		{"use_item без предмета", `{"outcome":"intent","verb":"use_item"}`},
		{"compare с одним фактом", `{"outcome":"intent","verb":"compare","facts":["f_x"]}`},
		{"examine без цели", `{"outcome":"intent","verb":"examine"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parse(t, c.reply, hint); got.Accepted() {
				t.Error("принято действие без обязательного аргумента")
			}
		})
	}
}

func TestLookNeedsNothing(t *testing.T) {
	if got := parse(t, `{"outcome":"intent","verb":"look"}`, harbourHint(t)); !got.Accepted() {
		t.Errorf("look отвергнут: %q", got.Clarify)
	}
}

// Требования уходят в промпт: без них модель не знает, что заполнять.
func TestPromptDeclaresArity(t *testing.T) {
	brief := arityBrief()
	for _, want := range []string{"talk_to", "question", "target", "topic", "move_zone", "node"} {
		if !strings.Contains(brief, want) {
			t.Errorf("в требованиях нет %q:\n%s", want, brief)
		}
	}
	if brief != arityBrief() {
		t.Error("требования нестабильны между вызовами — префикс промпта не закэшируется")
	}
}

// Грамматика свободного текста и грамматика команд обязаны совпадать: иначе
// один и тот же ввод ведёт себя по-разному в двух режимах.
func TestArityMatchesStructuredParser(t *testing.T) {
	for _, d := range core.AllVerbs() {
		r := requires(d.Verb)
		n := 0
		for _, b := range []bool{r.Target, r.Topic, r.Node, r.Item, r.Ability, r.Text} {
			if b {
				n++
			}
		}
		if n == 0 && r.Facts == 0 && d.Verb != "look" {
			t.Errorf("глагол %q не требует ничего — проверь таблицу арности", d.Verb)
		}
	}
}

// --- перечисления из сцены ---

// Инструкция «бери id из списка» слабой моделью игнорируется. Перечисление в
// схеме — не просьба, а грамматика.
func TestSchemaCarriesSceneEnums(t *testing.T) {
	hint := harbourHint(t)
	props := SchemaFor(hint)["properties"].(map[string]any)

	target := props["target"].(map[string]any)
	enum, ok := target["enum"].([]string)
	if !ok {
		t.Fatal("у target нет перечисления присутствующих")
	}
	if len(enum) != len(hint.Entities) {
		t.Errorf("в перечислении %d сущностей, в сцене %d", len(enum), len(hint.Entities))
	}
	for _, e := range hint.Entities {
		if !containsStr(enum, e.ID) {
			t.Errorf("в перечислении нет %q", e.ID)
		}
	}
	if _, ok := props["topic"].(map[string]any)["enum"]; !ok {
		t.Error("у topic нет перечисления известных тем")
	}
}

// Пустая сцена не должна давать пустой enum: пустое перечисление делает схему
// невыполнимой, и модель не сможет ответить вообще ничем.
func TestEmptySceneLeavesFieldsUnconstrained(t *testing.T) {
	props := SchemaFor(SceneHint{})["properties"].(map[string]any)
	for _, field := range []string{"target", "topic", "node"} {
		if _, ok := props[field].(map[string]any)["enum"]; ok {
			t.Errorf("у %q появилось пустое перечисление", field)
		}
	}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// --- раунд починки ---

// parserRepairing отвечает неполно на первой попытке и полно на второй.
func parserRepairing(t *testing.T, first, second string) (*Parser, *llm.Fake) {
	t.Helper()
	f := llm.NewFake("fake", true).ReplyWith(func(r llm.Request) string {
		if strings.Contains(r.Input, "Предыдущий ответ был неполон") {
			return second
		}
		return first
	})
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleIntentParser, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
	return NewParser(gw), f
}

// Ровно тот случай с живого прогона: «Поздороваться с Берном» дало talk_to
// без цели. Имя есть во фразе — значит цель разрешается на месте, без второго
// вызова модели.
func TestMissingTargetResolvedFromPlayerText(t *testing.T) {
	hint := harbourHint(t)
	p, f := parserRepairing(t,
		`{"outcome":"intent","verb":"talk_to"}`,
		`{"outcome":"intent","verb":"talk_to","target":"e_bern"}`)

	got, err := p.Parse(context.Background(), "Поздороваться с Берном", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Accepted() {
		t.Fatalf("цель не разрешилась по имени: %q", got.Clarify)
	}
	if string(got.Intent.Args.Target) != "e_bern" {
		t.Errorf("цель %q, ожидалась e_bern", got.Intent.Args.Target)
	}
	if n := len(f.Calls()); n != 1 {
		t.Errorf("вызовов %d — имя во фразе должно решаться без второго обращения", n)
	}
}

// Если имени во фразе нет, разрешать нечего — тогда работает раунд починки.
func TestRepairFiresWhenNameIsAbsentFromText(t *testing.T) {
	hint := harbourHint(t)
	p, f := parserRepairing(t,
		`{"outcome":"intent","verb":"talk_to"}`,
		`{"outcome":"intent","verb":"talk_to","target":"e_bern"}`)

	got, err := p.Parse(context.Background(), "поздороваться", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Accepted() {
		t.Fatalf("починка не помогла: %q", got.Clarify)
	}
	if n := len(f.Calls()); n != 2 {
		t.Errorf("вызовов %d, ожидалось 2", n)
	}
	if !strings.Contains(f.Calls()[1].Input, "к кому") {
		t.Errorf("в починку не попало пропущенное поле")
	}
}

// Второго раунда нет: дальше это уже не недопонимание.
func TestRepairHappensOnlyOnce(t *testing.T) {
	hint := harbourHint(t)
	p, f := parserRepairing(t,
		`{"outcome":"intent","verb":"talk_to"}`,
		`{"outcome":"intent","verb":"talk_to"}`)

	got, err := p.Parse(context.Background(), "поздороваться", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Accepted() {
		t.Fatal("принято действие без обязательного аргумента")
	}
	if n := len(f.Calls()); n != 2 {
		t.Errorf("вызовов %d, ожидалось ровно 2", n)
	}
}

// Ссылка на несуществующую сущность починке не подлежит: это не пропуск, а
// выдумка, и второй заход её не исправит.
func TestBadReferenceIsNotRepaired(t *testing.T) {
	hint := harbourHint(t)
	p, f := parserRepairing(t,
		`{"outcome":"intent","verb":"examine","target":"e_призрак"}`,
		`{"outcome":"intent","verb":"examine","target":"e_призрак"}`)
	got, err := p.Parse(context.Background(), "смотрю на призрака", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Accepted() {
		t.Fatal("принята выдуманная сущность")
	}
	if n := len(f.Calls()); n != 1 {
		t.Errorf("вызовов %d — выдумку чинить не надо", n)
	}
}

// Метрика считается по итогу, а не по попыткам: иначе починенный ввод
// записывался бы и как отказ, и как успех.
func TestRepairCountsOnceInMetrics(t *testing.T) {
	hint := harbourHint(t)
	target := hint.Entities[0].ID
	p, _ := parserRepairing(t,
		`{"outcome":"intent","verb":"talk_to"}`,
		`{"outcome":"intent","verb":"talk_to","target":"`+target+`"}`)
	p.Parse(context.Background(), "привет", hint, llm.Request{})

	m := p.Metrics()
	if m.Observations() != 1 {
		t.Errorf("наблюдений %d, ожидалось 1", m.Observations())
	}
	if r := m.Rate(core.ClassSocial); r != 0 {
		t.Errorf("починенный ввод записан отказом: %.2f", r)
	}
}

// Слова игрока обязаны доехать до персонажа. «Поздороваться с Берном»
// превращается в talk_to, который своего текста не несёт, — и актёр получал
// пустую реплику, классифицировал акт как «прочее» и отвечал не на
// приветствие, а ни на что.
func TestPlayerWordsSurviveIntoSocialIntent(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"talk_to","target":"e_bern"}`)
	gi := &GameInterpreter{Parser: p, Game: interpGame(t)}
	in, _, err := gi.Interpret(context.Background(), "Поздороваться с Берном", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if in == nil {
		t.Fatal("намерение не разобралось")
	}
	if in.Args.Text != "Поздороваться с Берном" {
		t.Errorf("слова игрока потеряны: %q", in.Args.Text)
	}
}

// У глаголов, где текст — это содержимое хода, разобранное моделью важнее
// исходной строки: theorize пишет в дневник гипотезу, а не команду.
func TestParsedTextWinsWhereItIsTheContent(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"theorize","text":"Токе лжёт про ночь"}`)
	gi := &GameInterpreter{Parser: p, Game: interpGame(t)}
	in, _, err := gi.Interpret(context.Background(), "запишу-ка мысль: Токе лжёт про ночь", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if in.Args.Text != "Токе лжёт про ночь" {
		t.Errorf("гипотеза подменена исходной строкой: %q", in.Args.Text)
	}
}

// interpGame — «Гавань» на старте: Берн и Нильс в сцене, известен один факт.
func interpGame(t *testing.T) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	return core.NewGame(*cfg)
}

// --- разбор в контексте разговора ---

// Разбор без контекста разговора не понимает ответа на свой же вопрос.
// Живой прогон: стражник просит предписание, игрок пишет «достаю из кармана»,
// парсер спрашивает «чем именно?», игрок отвечает «рукой» — и всё начинается
// заново, потому что о заданном вопросе никто не помнит.
func TestPromptCarriesTheConversation(t *testing.T) {
	p, f := parserWith(t, `{"outcome":"intent","verb":"say","text":"вот предписание"}`)
	hint := harbourHint(t)
	hint.Talk = Talk{
		With: "e_bern",
		Recent: []store.Exchange{
			{Player: "Поздороваться с Берном", Reply: "Добрый день. Что привело вас в такую погоду?", Turn: 1},
			{Player: "приехал по заданию — расследование", Reply: "Тогда предъявите предписание.", Turn: 2},
		},
		Pending: "чем именно?",
	}

	if _, err := p.Parse(context.Background(), "рукой", hint, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	in := f.Calls()[0].Input
	for _, want := range []string{"Тогда предъявите предписание", "e_bern", "чем именно?"} {
		if !strings.Contains(in, want) {
			t.Errorf("в промпте нет %q:\n%s", want, in)
		}
	}
	// Разговор обязан стоять до текущей фразы: промпт читается сверху вниз.
	if strings.Index(in, "предъявите предписание") > strings.Index(in, "рукой") {
		t.Errorf("разговор встал после фразы игрока:\n%s", in)
	}
}

// Без разговора промпт не должен обрастать пустыми заголовками: пустая
// секция это шум, за который платят токенами каждый ход.
func TestPromptWithoutConversationStaysClean(t *testing.T) {
	p, f := parserWith(t, `{"outcome":"intent","verb":"look"}`)
	if _, err := p.Parse(context.Background(), "осмотреться", harbourHint(t), llm.Request{}); err != nil {
		t.Fatal(err)
	}
	if in := f.Calls()[0].Input; strings.Contains(in, "Разговор") ||
		strings.Contains(in, "Ты спросил") {
		t.Errorf("пустая секция разговора попала в промпт:\n%s", in)
	}
}

// Модели прямо сказано, что игрок отвечает на заданный вопрос: иначе она
// читает «рукой» как новое действие и снова просит уточнить.
func TestPendingQuestionIsFramedAsAnAnswer(t *testing.T) {
	p, f := parserWith(t, `{"outcome":"unsupported","reason":"предмета нет"}`)
	hint := harbourHint(t)
	hint.Talk = Talk{Pending: "чем именно?"}
	if _, err := p.Parse(context.Background(), "рукой", hint, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	if in := f.Calls()[0].Input; !strings.Contains(in, "отвечает на этот вопрос") {
		t.Errorf("ответ на вопрос не помечен как ответ:\n%s", in)
	}
}

// Спрашивать «что именно ты хочешь сказать?» у того, кто только что это
// сказал, — допрос игрока о его же фразе. Строка ввода и есть текст.
func TestFreeTextVerbTakesThePlayersOwnWords(t *testing.T) {
	for _, verb := range []string{"say", "theorize", "emote"} {
		t.Run(verb, func(t *testing.T) {
			p, _ := parserWith(t, `{"outcome":"intent","verb":"`+verb+`"}`)
			res, err := p.Parse(context.Background(),
				"приехал по заданию руководства — расследование", harbourHint(t), llm.Request{})
			if err != nil {
				t.Fatal(err)
			}
			if !res.Accepted() {
				t.Fatalf("ход не принят, спросили: %q", res.Clarify)
			}
			if res.Intent.Args.Text != "приехал по заданию руководства — расследование" {
				t.Errorf("текст хода %q", res.Intent.Args.Text)
			}
		})
	}
}

// Модель, назвавшая текст сама, важнее исходной строки: в дневник пишется
// гипотеза, а не команда её записать.
func TestModelTextWinsOverRawInput(t *testing.T) {
	p, _ := parserWith(t,
		`{"outcome":"intent","verb":"theorize","text":"писарь врёт про контору"}`)
	res, err := p.Parse(context.Background(),
		"запишу-ка догадку: писарь врёт про контору", harbourHint(t), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent.Args.Text != "писарь врёт про контору" {
		t.Errorf("текст хода %q", res.Intent.Args.Text)
	}
}

// --- инструменты сцены ---

// Обязательный аргумент, которого в сцене взять негде, — тупик: игра
// спрашивает «чем именно?», а ответить нечем, потому что предметов тут нет.
// Живой прогон упирался в это на «достаю предписание из кармана».
func TestVerbWithUnsatisfiableSlotLeavesTheGrammar(t *testing.T) {
	bare := SchemaFor(SceneHint{})
	verbs := bare["properties"].(map[string]any)["verb"].(map[string]any)["enum"].([]string)
	for _, v := range verbs {
		if v == "use_item" {
			t.Error("use_item предложен там, где применять нечего")
		}
	}
}

// Там, где инструмент есть, глагол возвращается — и с перечислением, чтобы
// модель не могла выдумать предмет.
func TestToolsInSceneRestoreTheVerb(t *testing.T) {
	hint := SceneHint{Tools: []Named{{"p_hook_lamp", "Фонарь на крюке"}}}
	props := SchemaFor(hint)["properties"].(map[string]any)
	verbs := props["verb"].(map[string]any)["enum"].([]string)
	var found bool
	for _, v := range verbs {
		if v == "use_item" {
			found = true
		}
	}
	if !found {
		t.Error("инструмент в сцене есть, а применить его нельзя")
	}
	item := props["item"].(map[string]any)
	enum, ok := item["enum"].([]string)
	if !ok || len(enum) != 1 || enum[0] != "p_hook_lamp" {
		t.Errorf("предмет без перечисления: %v", item)
	}
	if !strings.Contains(hint.Render(), "p_hook_lamp") {
		t.Errorf("инструмент не назван в подсказке:\n%s", hint.Render())
	}
}

// Инструменты берутся из сцены: проп с меткой tool в текущем узле, и ничего
// кроме — иначе игрок «применяет» бочки.
func TestBuildHintCollectsToolsOfTheNode(t *testing.T) {
	g := harbourGame(t)
	if got := BuildHint(g).Tools; len(got) != 0 {
		t.Errorf("на пристани нашлись инструменты: %v", got)
	}
	g.Node = "n_forge"
	tools := BuildHint(g).Tools
	if len(tools) == 0 {
		t.Fatal("в кузнице нет инструментов — фикстура сломана")
	}
	for _, tool := range tools {
		if tool.ID != "p_hammers" {
			t.Errorf("инструментом сочли %q", tool.ID)
		}
	}
}

// Отказ читает игрок, а не разработчик. «В доступных действиях нет глагола
// для использования предметов» — это сообщение компилятора, а не мира.
func TestPromptDemandsWorldLanguageInRefusals(t *testing.T) {
	p, f := parserWith(t, `{"outcome":"unsupported","reason":"нечего применить"}`)
	if _, err := p.Parse(context.Background(), "достаю предписание",
		harbourHint(t), llm.Request{}); err != nil {
		t.Fatal(err)
	}
	sys := f.Calls()[0].System
	if !strings.Contains(sys, "языком мира") {
		t.Errorf("промпт не требует говорить с игроком языком мира:\n%s", sys)
	}
}

// Разговор идёт с конкретным человеком, и называть его в каждой фразе игрок
// не обязан — структурированный ввод это уже умеет. Живой прогон: «есть ли
// слухи?» посреди разговора с Берном получало «к кому или к чему?».
func TestMissingTargetFallsBackToInterlocutor(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"talk_to"}`)
	hint := harbourHint(t)
	hint.Talk = Talk{With: "e_bern"}

	res, err := p.Parse(context.Background(), "есть ли слухи?", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Accepted() {
		t.Fatalf("ход не принят: %q", res.Clarify)
	}
	if res.Intent.Args.Target != "e_bern" {
		t.Errorf("цель %q — собеседник не подставился", res.Intent.Args.Target)
	}
}

// Собеседник подставляется, только если он всё ещё в сцене: иначе игрок
// обращается к тому, кто ушёл.
func TestInterlocutorOutsideSceneIsNotSubstituted(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"talk_to"}`)
	hint := harbourHint(t)
	hint.Talk = Talk{With: "e_toke"} // писарь в конторе, а не на пристани

	res, err := p.Parse(context.Background(), "есть ли слухи?", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted() {
		t.Errorf("цель взялась из ниоткуда: %q", res.Intent.Args.Target)
	}
}

// Вопрос о том, чего парти не знает, — не отказ словаря, а разговор. Ровно
// для этого и заведён диалоговый актёр: он уклонится, намекнёт или спросит
// Мастера, а гейт фактов при этом не трогается.
func TestPromptTurnsUnknownTopicQuestionsIntoTalk(t *testing.T) {
	p, f := parserWith(t, `{"outcome":"intent","verb":"say","text":"кто тут ночами ходит?"}`)
	if _, err := p.Parse(context.Background(), "а кто тут ночами ходит?",
		harbourHint(t), llm.Request{}); err != nil {
		t.Fatal(err)
	}
	if sys := f.Calls()[0].System; !strings.Contains(sys, "это say") {
		t.Errorf("промпт не превращает вопрос вне банка тем в разговор:\n%s", sys)
	}
}

// Открытый вопрос собеседнику — разговор, а не допрос словаря. «Есть ли
// слухи?» темы в банке не имеет и иметь не может: это приглашение говорить,
// и отвечать на него должен персонаж.
func TestOpenQuestionToInterlocutorBecomesTalk(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"question","target":"e_bern"}`)
	hint := harbourHint(t)
	hint.Talk = Talk{With: "e_bern"}

	res, err := p.Parse(context.Background(),
		"есть ли какие-нибудь слухи в последнее время?", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Accepted() {
		t.Fatalf("ход не принят: %q", res.Clarify)
	}
	if res.Intent.Verb != "say" {
		t.Errorf("глагол %q — открытый вопрос должен стать разговором", res.Intent.Verb)
	}
	if res.Intent.Args.Target != "e_bern" || res.Intent.Args.Text == "" {
		t.Errorf("разговор без адресата или без слов: %+v", res.Intent.Args)
	}
}

// Названную тему подменять разговором нельзя: это настоящий ход
// расследования, и терять его ради живой реплики — прямая потеря игры.
func TestNamedTopicSurvivesAsAQuestion(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"question","target":"e_bern"}`)
	hint := harbourHint(t)
	hint.Talk = Talk{With: "e_bern"}

	res, err := p.Parse(context.Background(),
		"спрошу про тело Халдена на складе", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent == nil || res.Intent.Verb != "question" {
		t.Fatalf("ход расследования потерян: %+v", res)
	}
	if res.Intent.Args.Topic != "f_body_found" {
		t.Errorf("тема %q — названная тема не найдена в фразе", res.Intent.Args.Topic)
	}
}

// Без собеседника подменять нечем: вопрос в воздух остаётся вопросом игроку.
func TestOpenQuestionWithoutInterlocutorStillAsks(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"question","target":"e_bern"}`)
	res, err := p.Parse(context.Background(), "есть ли слухи?", harbourHint(t), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted() {
		t.Errorf("ход принят без темы и без собеседника: %+v", res.Intent)
	}
}

// Примеры в промпте сильнее правил: пример, учивший отвечать clarify на
// вопрос вне банка тем, переучивал модель обратно ровно там, где правило
// требует разговора.
func TestExamplesAgreeWithTheRules(t *testing.T) {
	p, f := parserWith(t, `{"outcome":"intent","verb":"look"}`)
	if _, err := p.Parse(context.Background(), "осмотреться", harbourHint(t), llm.Request{}); err != nil {
		t.Fatal(err)
	}
	sys := f.Calls()[0].System
	if strings.Contains(sys, `"clarify":"Об этом парти пока ничего не знает."`) {
		t.Error("в примерах остался clarify на вопрос вне банка тем")
	}
	if !strings.Contains(sys, `"verb":"say","target":"e_ivar"`) {
		t.Error("нет примера, где вопрос вне банка тем становится разговором")
	}
}

// --- предъявление предмета ---

// Исходный баг: «показать предписание» понять было нечем. Предмет в кармане
// обязан попасть в контекст сцены, иначе глагол предъявления не появится в
// грамматике вовсе.
func TestCarriedItemsReachTheHint(t *testing.T) {
	g := harbourGame(t)
	g.DB.Items["i_writ"] = store.Item{ID: "i_writ", Kind: "credential",
		Name: "Предписание магистрата"}
	g.Acquire("i_writ")

	hint := BuildHint(g)
	if len(hint.Carried) != 1 || hint.Carried[0].ID != "i_writ" {
		t.Fatalf("инвентарь не доехал до подсказки: %+v", hint.Carried)
	}
	if !strings.Contains(hint.Render(), "i_writ") {
		t.Errorf("предмет не назван в промпте:\n%s", hint.Render())
	}
}

// Глагол предъявления появляется ровно тогда, когда есть что предъявить, — и
// предмет обязан быть в перечислении, чтобы модель не могла назвать чужой.
func TestPresentEntersGrammarWithCarriedItems(t *testing.T) {
	bare := SchemaFor(SceneHint{})["properties"].(map[string]any)
	for _, v := range bare["verb"].(map[string]any)["enum"].([]string) {
		if v == "present" {
			t.Error("предъявление предложено там, где предъявлять нечего")
		}
	}

	hint := SceneHint{Carried: []Named{{"i_writ", "Предписание магистрата"}}}
	props := SchemaFor(hint)["properties"].(map[string]any)
	var found bool
	for _, v := range props["verb"].(map[string]any)["enum"].([]string) {
		if v == "present" {
			found = true
		}
	}
	if !found {
		t.Error("есть что предъявить, а глагола предъявления нет")
	}
	enum, ok := props["item"].(map[string]any)["enum"].([]string)
	if !ok || len(enum) != 1 || enum[0] != "i_writ" {
		t.Errorf("носимый предмет не попал в перечисление: %v", props["item"])
	}
}

// Носимая бумага НЕ должна возвращать в грамматику use_item: он резолвится
// только по пропам узла с меткой tool, и глагол с ненаходимым предметом
// уезжает в бросок по пропу, которого нет.
func TestCarriedItemsDoNotReviveUseItem(t *testing.T) {
	hint := SceneHint{Carried: []Named{{"i_writ", "Предписание магистрата"}}}
	for _, v := range SchemaFor(hint)["properties"].(map[string]any)["verb"].(map[string]any)["enum"].([]string) {
		if v == "use_item" {
			t.Error("применение инструмента вернулось в грамматику из-за носимой бумаги")
		}
	}
}

// Предмет игрок называет словами, а не идентификатором. «Показать предписание»
// обязано разобраться без уточнения — это и был исходный баг.
func TestItemNameResolvesFromThePlayersWords(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"present","target":"e_bern"}`)
	hint := harbourHint(t)
	hint.Carried = []Named{{"i_writ", "Предписание магистрата"}}

	res, err := p.Parse(context.Background(), "показать предписание Берну", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Accepted() {
		t.Fatalf("ход не принят: %q", res.Clarify)
	}
	if res.Intent.Args.Item != "i_writ" {
		t.Errorf("предмет %q — имя не разрешилось", res.Intent.Args.Item)
	}
	if res.Intent.Verb != "present" || res.Intent.Args.Target != "e_bern" {
		t.Errorf("ход разобрался как %+v", res.Intent.Args)
	}
}

// Предъявление без предмета — не действие: предъявлять надо что-то.
func TestPresentWithoutItemIsNotAccepted(t *testing.T) {
	p, _ := parserWith(t, `{"outcome":"intent","verb":"present","target":"e_bern"}`)
	hint := harbourHint(t)
	hint.Carried = []Named{{"i_writ", "Предписание магистрата"}}

	res, err := p.Parse(context.Background(), "предъявить кое-что", hint, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted() {
		t.Errorf("предъявление принято без предмета: %+v", res.Intent.Args)
	}
}

// В промпте есть пример предъявления: правило без примера модель переучивает
// обратно — это уже проверено на вопросах вне банка тем.
func TestPromptShowsHowToPresent(t *testing.T) {
	p, f := parserWith(t, `{"outcome":"intent","verb":"look"}`)
	if _, err := p.Parse(context.Background(), "осмотреться", harbourHint(t), llm.Request{}); err != nil {
		t.Fatal(err)
	}
	if sys := f.Calls()[0].System; !strings.Contains(sys, `"verb":"present"`) {
		t.Errorf("в примерах нет предъявления:\n%s", sys)
	}
}
