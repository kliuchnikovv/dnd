package scenegen

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

func genGateway(reply string) *llm.Gateway {
	f := llm.NewFake("fake", true).ReplyWith(func(llm.Request) string { return reply })
	return llm.NewGateway(
		llm.NewRouter().Route(llm.RoleWorldsmith, llm.Target{Provider: f, Model: "claude-sonnet-5"}),
		llm.NewLedger(llm.Caps{}))
}

// Валидный ответ модели проходит repair+validate и принимается за одну попытку.
func TestGenerateAcceptsValidSpec(t *testing.T) {
	raw, _ := json.Marshal(validHoldSpec())
	spec, tr, err := Generate(context.Background(), genGateway(string(raw)), "гость к ночи", "hold")
	if err != nil {
		t.Fatalf("валидная сцена отвергнута: %v", err)
	}
	if spec == nil || !tr.OK || len(tr.Attempts) != 1 {
		t.Fatalf("след генерации неверен: ok=%v attempts=%d", tr.OK, len(tr.Attempts))
	}
	if spec.Mode != "hold" {
		t.Errorf("режим спека потерян: %q", spec.Mode)
	}
}

// Битый ответ ретраится ≤3 раза и, не выправившись, даёт ошибку (сцену не пускаем).
func TestGenerateRetriesThenFailsOnGarbage(t *testing.T) {
	_, tr, err := Generate(context.Background(), genGateway("это не сцена и не json"), "", "hold")
	if err == nil {
		t.Fatal("битая генерация прошла как валидная")
	}
	if tr.OK || len(tr.Attempts) != 3 {
		t.Fatalf("ожидалось 3 неуспешные попытки, получено ok=%v attempts=%d", tr.OK, len(tr.Attempts))
	}
}

// Без шлюза — ошибка (сцену из воздуха не выдумываем).
func TestGenerateNilGatewayErrors(t *testing.T) {
	if _, _, err := Generate(context.Background(), nil, "тема", "hold"); err == nil {
		t.Fatal("nil-шлюз не дал ошибку")
	}
}
