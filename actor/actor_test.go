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
	a, _ := actorWith(t, `{"line":"«Я ничего не видел.»"}`)
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
	a, f := actorWith(t, `{"line":"Ничего."}`)
	sp, ok := SpeakerFor(g, "e_bern")
	if !ok {
		t.Fatal("у Берна нет голоса — фикстура сломана")
	}
	if _, err := a.Line(context.Background(), sp,
		Situation{Verb: "talk_to", Known: KnownTopics(g)}, llm.Request{}); err != nil {
		t.Fatal(err)
	}
	prompt := f.Calls()[0].Input
	known := KnownTopics(g)
	for _, k := range known {
		if !strings.Contains(prompt, k) {
			t.Errorf("в промпте нет известной темы %q", k)
		}
	}
	// Токены правильного ответа не должны попасть в промпт ни под каким видом.
	for _, leak := range []string{"seal_cord", "night_before_tide", "audit_shortfall", "redacted"} {
		if strings.Contains(prompt, leak) {
			t.Errorf("промпт содержит %q", leak)
		}
	}
	if !strings.Contains(f.Calls()[0].System, "не сообщать факты") &&
		!strings.Contains(f.Calls()[0].System, "сообщать факты") {
		t.Error("в системном промпте нет запрета сообщать факты")
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
	a, f := actorWith(t, `{"line":"Отойдите."}`)
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
	v, _ := voicer(t, g, `{"line":"Мокро сегодня."}`)
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
	v, f := voicer(t, g, `{"line":"не должно прозвучать"}`)
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
			v, f := voicer(t, g, `{"line":"тишина"}`)
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
