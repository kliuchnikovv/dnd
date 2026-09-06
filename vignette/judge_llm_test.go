package vignette

import (
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

func judgeGateway(reply string) *llm.Gateway {
	f := llm.NewFake("fake", true).ReplyWith(func(llm.Request) string { return reply })
	return llm.NewGateway(
		llm.NewRouter().Route(llm.RoleIntentParser, llm.Target{Provider: f, Model: "claude-haiku-4-5"}),
		llm.NewLedger(llm.Caps{}))
}

// LLM-судья разбирает JSON-вердикт модели в Ruling по каноничной шкале.
func TestLLMJudgeParsesVerdict(t *testing.T) {
	gw := judgeGateway(`{"kind":"listen","target":"door","stat":"edge","difficulty":"hard","vantage":"disadvantage"}`)
	r := NewLLMJudge(gw).Rule("прислушиваюсь", jview())
	if r.Kind != "listen" || r.Target != "door" {
		t.Fatalf("вердикт разобран неверно: %+v", r)
	}
	if r.DC != DCHard {
		t.Errorf("DC=%d, ожидался hard=%d (каноничная шкала)", r.DC, DCHard)
	}
	if r.Adv != -1 {
		t.Errorf("vantage disadvantage не дал adv=-1: %d", r.Adv)
	}
}

// Битый/пустой ответ модели → откат на KeywordJudge (игра не встаёт).
func TestLLMJudgeFallsBackOnGarbage(t *testing.T) {
	gw := judgeGateway("это не json вовсе")
	r := NewLLMJudge(gw).Rule("осматриваю дверь", jview())
	if r.Kind != "look" { // как решил бы KeywordJudge
		t.Fatalf("откат на keyword не сработал: %+v", r)
	}
}

// Без шлюза (nil) — сразу keyword-фолбэк.
func TestLLMJudgeNilGatewayFallsBack(t *testing.T) {
	r := NewLLMJudge(nil).Rule("иду вперёд", jview())
	if r.Kind != "moveon" {
		t.Fatalf("nil-шлюз не дал keyword-фолбэк: %+v", r)
	}
}
