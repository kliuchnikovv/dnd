package actor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/llm"
)

// Guard — проверка реплики на выдумку. Задача проще генерации: сверить одну
// короткую фразу с коротким списком материала. Поэтому её можно отдать
// дешёвой модели, а закрытый набор ходов и шаблонный откат делают её ошибку
// нестрашной.
type Guard struct {
	gw     *llm.Gateway
	schema string
}

const guardPrompt = `Ты проверяешь реплику персонажа игры на выдумку.

Ниже материал — всё, что персонаж знает и вправе сказать. Ответь, утверждает
ли реплика что-либо, чего в материале нет: имена, места, должности, числа,
события, учреждения, порядки.

Не считается выдумкой: вежливость, уклонение, отказ, встречный вопрос, эмоция,
пересказ материала другими словами, упоминание себя и собеседника.

Считается выдумкой: любой новый факт о мире, даже мелкий и правдоподобный.

Верни {"invented": true|false, "what": "что именно придумано"}.`

func NewGuard(gw *llm.Gateway) *Guard {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"invented"},
		"properties": map[string]any{
			"invented": map[string]any{"type": "boolean"},
			"what":     map[string]any{"type": "string"},
		},
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return &Guard{gw: gw, schema: string(raw)}
}

// Check возвращает true, если реплику можно показывать.
func (g *Guard) Check(ctx context.Context, line string, material []string, req llm.Request) (bool, error) {
	req.Role = llm.RoleCanonGuard
	req.Schema = g.schema
	req.System = guardPrompt
	req.MaxTokens = 200

	var b strings.Builder
	b.WriteString("Материал:\n")
	for _, m := range material {
		if strings.TrimSpace(m) != "" {
			b.WriteString("  - " + m + "\n")
		}
	}
	b.WriteString("\nРеплика: " + line + "\n")
	req.Input = b.String()

	resp, err := g.gw.Do(ctx, req)
	if err != nil {
		return false, err
	}
	var out struct {
		Invented bool   `json:"invented"`
		What     string `json:"what"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil {
		return false, fmt.Errorf("actor: проверка не разобралась: %w", err)
	}
	return !out.Invented, nil
}
