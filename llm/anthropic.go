package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicProvider — единственное место в проекте, где живёт сетевой вызов
// к модели. Гарантию схемы даёт strict tool use: инструмент со strict:true и
// additionalProperties:false плюс принудительный выбор этого инструмента.
// Это не «попросили вернуть JSON», а ограничение сэмплирования.
type anthropicProvider struct {
	client anthropic.Client
	// toolName — имя инструмента, через который уходит структурированный
	// ответ. Стабильно: имя входит в кэшируемую грамматику.
	toolName string
}

// NewAnthropic собирает провайдер. Без опций ключ берётся из окружения по
// правилам SDK, поэтому вызывающему не нужно знать, откуда он взялся.
func NewAnthropic(opts ...option.RequestOption) Provider {
	return &anthropicProvider{client: anthropic.NewClient(opts...), toolName: "emit"}
}

func (a *anthropicProvider) Name() string { return "anthropic" }

// StrictOutput истинно: SDK ограничивает сэмплирование грамматикой схемы,
// поэтому провайдер допустим для ролей, мутирующих состояние.
func (a *anthropicProvider) StrictOutput() bool { return true }

func (a *anthropicProvider) Complete(ctx context.Context, model string, r Request) (Response, error) {
	maxTokens := int64(r.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(r.Input)),
		},
	}
	if r.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: r.System}}
	}
	if r.Schema != "" {
		tool, err := a.schemaTool(r.Schema)
		if err != nil {
			return Response{}, err
		}
		params.Tools = []anthropic.ToolUnionParam{{OfTool: tool}}
		params.ToolChoice = anthropic.ToolChoiceParamOfTool(a.toolName)
	}

	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, fmt.Errorf("llm: вызов модели %s: %w", model, err)
	}

	resp := Response{Usage: Usage{
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}}

	// При заявленной схеме ответом считается вход инструмента, а не проза:
	// именно он ограничен грамматикой.
	if r.Schema != "" {
		for _, block := range msg.Content {
			if use, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				resp.Text = string(use.Input)
				return resp, nil
			}
		}
		return Response{}, fmt.Errorf("llm: модель %s не вернула структурированный ответ", model)
	}

	var b strings.Builder
	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(text.Text)
		}
	}
	resp.Text = b.String()
	return resp, nil
}

// schemaTool превращает JSON-схему в определение инструмента. Схема приходит
// строкой, потому что её автор — вызывающий слой, а не этот пакет.
func (a *anthropicProvider) schemaTool(schema string) (*anthropic.ToolParam, error) {
	var s struct {
		Properties any      `json:"properties"`
		Required   []string `json:"required"`
	}
	if err := json.Unmarshal([]byte(schema), &s); err != nil {
		return nil, fmt.Errorf("llm: схема не разобралась: %w", err)
	}
	if s.Properties == nil {
		return nil, fmt.Errorf("llm: в схеме нет properties")
	}
	return &anthropic.ToolParam{
		Name:   a.toolName,
		Strict: anthropic.Bool(true),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: s.Properties,
			Required:   s.Required,
			// Без этого strict не даёт гарантии: модель вправе добавить поле.
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}, nil
}
