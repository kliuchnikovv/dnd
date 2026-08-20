package llm

import (
	"encoding/json"
	"testing"
)

func provider(t *testing.T) *anthropicProvider {
	t.Helper()
	p, ok := NewAnthropic().(*anthropicProvider)
	if !ok {
		t.Fatal("NewAnthropic вернул не тот тип")
	}
	return p
}

func TestAnthropicSatisfiesProviderAndGuaranteesSchema(t *testing.T) {
	var _ Provider = NewAnthropic()
	if !NewAnthropic().StrictOutput() {
		t.Error("провайдер обязан заявлять гарантию схемы, иначе парсер к нему не допустят")
	}
}

// Гарантия strict без additionalProperties:false неполна — модель вправе
// добавить поле, и оно молча приедет дальше.
func TestSchemaToolForbidsExtraProperties(t *testing.T) {
	p := provider(t)
	tool, err := p.schemaTool(`{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !tool.Strict.Valid() || tool.Strict.Value != true {
		t.Error("strict не выставлен")
	}
	if tool.InputSchema.ExtraFields["additionalProperties"] != false {
		t.Errorf("additionalProperties = %v, ожидалось false", tool.InputSchema.ExtraFields["additionalProperties"])
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "a" {
		t.Errorf("required = %v", tool.InputSchema.Required)
	}
	if tool.Name == "" {
		t.Error("у инструмента нет имени — имя входит в кэшируемую грамматику")
	}
}

func TestSchemaToolRejectsBrokenSchema(t *testing.T) {
	p := provider(t)
	if _, err := p.schemaTool(`{не json`); err == nil {
		t.Error("битая схема принята")
	}
	if _, err := p.schemaTool(`{"type":"object"}`); err == nil {
		t.Error("схема без properties принята — модели нечего заполнять")
	}
}

// Схема, которую реально отправляет парсер, обязана конвертироваться.
// Проверяется здесь, а не в intent, чтобы поймать расхождение форм.
func TestSchemaToolAcceptsRealisticIntentSchema(t *testing.T) {
	realistic := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"outcome"},
		"properties": map[string]any{
			"outcome": map[string]any{"type": "string", "enum": []string{"intent", "clarify"}},
			"verb":    map[string]any{"type": "string", "enum": []string{"question", "examine"}},
			"target":  map[string]any{"type": "string"},
		},
	}
	raw, err := json.Marshal(realistic)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := provider(t).schemaTool(string(raw))
	if err != nil {
		t.Fatalf("схема парсера не конвертируется: %v", err)
	}
	// Инструмент должен уходить в API сериализуемым.
	if _, err := json.Marshal(tool); err != nil {
		t.Errorf("инструмент не сериализуется: %v", err)
	}
}
