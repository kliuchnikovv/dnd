package rubric

// Тесты на проводку LLM-судьи: убеждаемся, что (а) промпт строится
// item-specific, (б) рубрика применяет парсер JSON-ответа, (в) ошибка
// провайдера/схемы даёт NA, а не pass/fail. Реальной модели в этих тестах
// нет — провайдер llm.Fake, и это хорошо: ошибки проводки ловятся без сети
// и трат.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/vignette"
)

// makeGW — линейный router (одна цель), через llm.NewSource(0) для
// детерминированной цены. Не используем реальный runtime — нам нужна
// только проводка.
//
// Шлюз ругается на отсутствие цены модели в таблице (это by-design —
// тратить по неизвестной цене нельзя; см. cmd/server/narrator.go), поэтому
// регистрируем дешёвую цену для stub-модели.
func makeGW(t *testing.T, provider llm.Provider) *llm.Gateway {
	t.Helper()
	const model = "stub-rubric"
	llm.SetPrice(model, llm.Price{InputMicroPerMTok: 100_000, OutputMicroPerMTok: 100_000})
	t.Cleanup(func() {
		// Цены живут в глобальной мапе — у теста нет нормального
		// "удалить", но это стенд-in для теста, и мы хотим детерминизма:
		// пока SetPrice есть, поверх всегда задаём одно и то же.
	})
	router := llm.NewRouter().Route(llm.RoleRubricJudge, llm.Target{Provider: provider, Model: model})
	return llm.NewGateway(router, llm.NewLedger(llm.Caps{GlobalDailyMicro: 1_000_000, ForecastDailyMicro: 250_000, PerTurnCalls: 8}))
}

func TestGwJudgeParsesPass(t *testing.T) {
	p := llm.NewFake("fake", false).ReplyWith(func(_ llm.Request) string {
		return `{"outcome":"pass","reason":"мастер принял заявку и приземлил её"}`
	})
	j := NewGatewayJudge(makeGW(t, p))
	v := j.Judge(context.Background(), A1, TurnRecord{Input: "спрыгиваю с крыши", MasterOutput: "ты летишь вниз, ветер свистит."})
	if v.Outcome != OutcomePass {
		t.Fatalf("want pass, got %v reason=%q", v.Outcome, v.Reason)
	}
	if v.Item != A1 {
		t.Errorf("item не пробрасывался в Verdict: %q", v.Item)
	}
	if v.Source != KindLLM {
		t.Errorf("источник должен быть LLM, got %v", v.Source)
	}
	if !strings.Contains(v.Reason, "призем") {
		t.Errorf("reason потерялся: %q", v.Reason)
	}
}

func TestGwJudgeParsesFail(t *testing.T) {
	p := llm.NewFake("fake", false).ReplyWith(func(_ llm.Request) string {
		return "{\"outcome\":\"fail\",\"reason\":\"отбил заявку — вахтёр\"}"
	})
	j := NewGatewayJudge(makeGW(t, p))
	v := j.Judge(context.Background(), A1, TurnRecord{Input: "делаю сальто", MasterOutput: "так нельзя, выбери из списка."})
	if v.Outcome != OutcomeFail {
		t.Fatalf("want fail, got %v reason=%q", v.Outcome, v.Reason)
	}
	if !strings.Contains(v.Reason, "вахт") {
		t.Errorf("reason потерялся: %q", v.Reason)
	}
}

func TestGwJudgeParsesNA(t *testing.T) {
	p := llm.NewFake("fake", false).ReplyWith(func(_ llm.Request) string {
		return `{"outcome":"na","reason":"нет броска в ходе"}`
	})
	j := NewGatewayJudge(makeGW(t, p))
	v := j.Judge(context.Background(), B5, TurnRecord{Input: "молчу", MasterOutput: "ты молчишь."})
	if v.Outcome != OutcomeNA {
		t.Fatalf("want NA, got %v reason=%q", v.Outcome, v.Reason)
	}
}

func TestGwJudgeAcceptsMarkdownWrap(t *testing.T) {
	// Модель часто оборачивает JSON в ```json ... ``` — парсер должен это
	// переварить.
	p := llm.NewFake("fake", false).ReplyWith(func(_ llm.Request) string {
		return "вот моя оценка:\n```json\n{\"outcome\":\"pass\",\"reason\":\"ок\"}\n```"
	})
	j := NewGatewayJudge(makeGW(t, p))
	v := j.Judge(context.Background(), A1, TurnRecord{Input: "х", MasterOutput: "y"})
	if v.Outcome != OutcomePass {
		t.Fatalf("markdown-wrap не распарсился: %v reason=%q", v.Outcome, v.Reason)
	}
}

func TestGwJudgeHandlesProviderError(t *testing.T) {
	p := llm.NewFake("fake", false).FailWith(errors.New("provider down"))
	j := NewGatewayJudge(makeGW(t, p))
	v := j.Judge(context.Background(), A1, TurnRecord{Input: "x", MasterOutput: "y"})
	if v.Outcome != OutcomeNA {
		t.Fatalf("ошибка провайдера должна давать NA, got %v", v.Outcome)
	}
	if !strings.Contains(v.Reason, "provider down") {
		t.Errorf("reason не объяснил ошибку: %q", v.Reason)
	}
}

func TestGwJudgeHandlesGarbageResponse(t *testing.T) {
	p := llm.NewFake("fake", false).ReplyWith(func(_ llm.Request) string {
		return "не-json-ответ, который не парсится"
	})
	j := NewGatewayJudge(makeGW(t, p))
	v := j.Judge(context.Background(), A1, TurnRecord{Input: "x", MasterOutput: "y"})
	if v.Outcome != OutcomeNA {
		t.Fatalf("мусорный ответ должен давать NA, got %v", v.Outcome)
	}
	if !strings.Contains(strings.ToLower(v.Reason), "json") &&
		!strings.Contains(strings.ToLower(v.Reason), "decode") &&
		!strings.Contains(strings.ToLower(v.Reason), "парс") {
		t.Errorf("reason не указал на парсинг: %q", v.Reason)
	}
}

func TestGwJudgeIncludesRollInPrompt(t *testing.T) {
	calls := make(chan int, 1)
	p := llm.NewFake("fake", false).ReplyWith(func(_ llm.Request) string {
		calls <- 1
		return `{"outcome":"pass","reason":"ок"}`
	})
	j := NewGatewayJudge(makeGW(t, p))
	j.Judge(context.Background(), B5, TurnRecord{
		Input:        "осмотр",
		MasterOutput: "броска не было, ты смотришь.",
		Roll: struct {
			D20, Mod, DC, Total, Margin int
			Natural                     int
			Line                        string
		}{D20: 18, Mod: 2, DC: 15, Total: 20, Margin: 5, Line: "бросок: d20=18+2 = 20 против 15 · +5"},
	})
	// Достать записанный запрос.
	for _, p := range p.Calls() {
		if !strings.Contains(p.Input, "бросок") || !strings.Contains(p.Input, "+5") {
			t.Errorf("промпт не содержит roll данных: %q", p.Input)
		}
		if !strings.Contains(p.Input, "осмотр") {
			t.Errorf("промпт не содержит ввод игрока: %q", p.Input)
		}
	}
}

func TestGwJudgeSystemMentionsItemAnchor(t *testing.T) {
	captured := make(chan llm.Request, 1)
	p := llm.NewFake("fake", false).ReplyWith(func(r llm.Request) string {
		captured <- r
		return `{"outcome":"pass","reason":"ок"}`
	})
	j := NewGatewayJudge(makeGW(t, p))
	j.Judge(context.Background(), A1, TurnRecord{Input: "x", MasterOutput: "y"})
	r := <-captured
	if !strings.Contains(r.System, "A1") || !strings.Contains(strings.ToLower(r.System), "якорь") {
		t.Errorf("system-prompt не содержит якорь A1: %q", r.System)
	}
	if !strings.Contains(r.System, "JSON") {
		t.Errorf("system-prompt не просит JSON: %q", r.System)
	}
	if r.Role != llm.RoleRubricJudge {
		t.Errorf("роль должна быть RoleRubricJudge, got %q", r.Role)
	}
}

// Rubric smoke: подключили GatewayJudge → мягкие пункты перестают быть NA.
func TestRubricWithLLMFillsSoftItems(t *testing.T) {
	sc := minScene()
	recs, _ := RunVignette(context.Background(), sc, []ScriptStep{
		{Input: "слушаю дверь", DiceValues: []int{15}},
	}, RunOptions{Judge: vignette.KeywordJudge{}, Narrator: scriptNarrator(func(_ vignette.Result, _ TurnRecord) string {
		return "ты слушаешь дверь. бросок: d20=15+2 = 17 против 15 · +2. голос знаком."
	})})
	_ = recs

	// Провайдер на любой запрос «pass» — этим подменяем реальную модель.
	p := llm.NewFake("fake", false).ReplyWith(func(_ llm.Request) string {
		return `{"outcome":"pass","reason":"хорошо"}`
	})
	gwJudge := NewGatewayJudge(makeGW(t, p))
	r := NewRubric().WithLLM(gwJudge)
	// A1 — мягкий, после подмены должен перестать быть NA.
	v := r.Judge(context.Background(), A1, recs[0])
	if v.Outcome == OutcomeNA {
		t.Errorf("A1 после подмены LLM-судьи должен оцениваться, got NA reason=%q", v.Reason)
	}
	if v.Source != KindLLM {
		t.Errorf("источник после подмены: %v", v.Source)
	}
}

// ApplyConcurrent должен давать тот же порядок и те же вердикты, что и
// Apply, — иначе отчёт по двум режимам нельзя сравнивать.
func TestApplyConcurrentMatchesApply(t *testing.T) {
	sc := minScene()
	recs, _ := RunVignette(context.Background(), sc, []ScriptStep{
		{Input: "слушаю дверь", DiceValues: []int{15}},
	}, RunOptions{Judge: vignette.KeywordJudge{}, Narrator: scriptNarrator(func(_ vignette.Result, _ TurnRecord) string {
		return "ты слушаешь дверь. бросок: d20=15+2 = 17 против 15 · +2. голос знаком."
	})})
	p := llm.NewFake("fake", false).ReplyWith(func(_ llm.Request) string {
		return `{"outcome":"pass","reason":"ок"}`
	})
	r := NewRubric().WithLLM(NewGatewayJudge(makeGW(t, p)))
	seq := r.Apply(context.Background(), recs[0])
	par := r.ApplyConcurrent(context.Background(), recs[0])
	if len(seq) != len(par) {
		t.Fatalf("длины разошлись: %d vs %d", len(seq), len(par))
	}
	for i := range seq {
		if seq[i].Item != par[i].Item {
			t.Errorf("позиция %d: item %s vs %s", i, seq[i].Item, par[i].Item)
		}
		if seq[i].Outcome != par[i].Outcome {
			t.Errorf("пункт %s: последовательный %v, параллельный %v",
				seq[i].Item, seq[i].Outcome, par[i].Outcome)
		}
	}
}
