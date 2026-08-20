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

func harbourHint(t *testing.T) SceneHint {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	return BuildHint(core.NewGame(*cfg))
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

func TestSchemaEnumeratesEveryVerb(t *testing.T) {
	s := Schema()
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
		{"theorize без текста", `{"outcome":"intent","verb":"theorize"}`},
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
