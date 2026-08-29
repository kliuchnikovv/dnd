package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const openRouterBase = "https://openrouter.ai/api/v1"

// openRouter — провайдер через OpenRouter. Реализован сырым HTTP намеренно:
// у OpenRouter нет официального Go SDK, а тянуть OpenAI-совместимую
// библиотеку ради одного эндпоинта дороже, чем сорок строк.
//
// Ценность OpenRouter здесь в том, что он даёт перебор моделей одним
// интерфейсом — это нужно для eval-сравнений и для смены модели без правки
// кода. Цена — гарантия схемы слабее, чем у грамматики: см. StrictOutput.
type openRouter struct {
	key     string
	baseURL string
	client  *http.Client
	strict  bool
	// title и referer — необязательная атрибуция приложения в OpenRouter.
	title string
}

type OROption func(*openRouter)

func ORWithAPIKey(key string) OROption         { return func(o *openRouter) { o.key = key } }
func ORWithBaseURL(url string) OROption        { return func(o *openRouter) { o.baseURL = url } }
func ORWithHTTPClient(c *http.Client) OROption { return func(o *openRouter) { o.client = c } }
func ORWithTitle(t string) OROption            { return func(o *openRouter) { o.title = t } }

// ORWithStrictOutput объявляет, доверяем ли мы гарантии схемы у этого
// маршрута. Влияет на допуск к ролям, мутирующим состояние.
func ORWithStrictOutput(v bool) OROption { return func(o *openRouter) { o.strict = v } }

// NewOpenRouter собирает провайдер. Ключ по умолчанию берётся из
// OPENROUTER_API_KEY — в коде и в репозитории его быть не должно.
func NewOpenRouter(opts ...OROption) Provider {
	o := &openRouter{
		key:     os.Getenv("OPENROUTER_API_KEY"),
		baseURL: openRouterBase,
		client:  &http.Client{Timeout: 90 * time.Second},
		strict:  true,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *openRouter) Name() string { return "openrouter" }

// StrictOutput. OpenRouter принимает response_format с json_schema и
// strict:true, но фактическая строгость зависит от модели, на которую он
// маршрутизирует, — это слабее грамматики, ограничивающей сэмплирование.
//
// Здесь по умолчанию true, и вот почему это не безрассудство: слой intent
// проверяет КАЖДОЕ значение ответа против сцены и реестра глаголов, а
// невалидный ответ превращает в вопрос игроку, а не в мутацию. То есть
// гарантия схемы у нас не несущая — несущая проверка значений.
// Кто хочет прежнюю жёсткость, ставит ORWithStrictOutput(false).
func (o *openRouter) StrictOutput() bool { return o.strict }

type orMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type orRequest struct {
	Model          string        `json:"model"`
	Messages       []orMessage   `json:"messages"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat *orRespFormat `json:"response_format,omitempty"`
	Usage          *orUsageOpt   `json:"usage,omitempty"`
	Stream         bool          `json:"stream,omitempty"`
}

type orUsageOpt struct {
	Include bool `json:"include"`
}

type orRespFormat struct {
	Type       string       `json:"type"`
	JSONSchema orJSONSchema `json:"json_schema"`
}

type orJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type orResponse struct {
	Choices []struct {
		Message      orMessage `json:"message"`
		FinishReason string    `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		Cost             float64 `json:"cost"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error"`
}

func (o *openRouter) Complete(ctx context.Context, model string, r Request) (Response, error) {
	if o.key == "" {
		return Response{}, fmt.Errorf("llm: OPENROUTER_API_KEY не задан")
	}

	body := orRequest{
		Model:     model,
		MaxTokens: r.MaxTokens,
		// Просим вернуть фактическую стоимость: OpenRouter маршрутизирует на
		// разных поставщиков, поэтому статическая таблица цен здесь врёт.
		Usage: &orUsageOpt{Include: true},
	}
	if r.System != "" {
		body.Messages = append(body.Messages, orMessage{Role: "system", Content: r.System})
	}
	body.Messages = append(body.Messages, orMessage{Role: "user", Content: r.Input})

	if r.Schema != "" {
		var schema map[string]any
		if err := json.Unmarshal([]byte(r.Schema), &schema); err != nil {
			return Response{}, fmt.Errorf("llm: схема не разобралась: %w", err)
		}
		body.ResponseFormat = &orRespFormat{
			Type:       "json_schema",
			JSONSchema: orJSONSchema{Name: "intent", Strict: true, Schema: schema},
		}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+o.key)
	req.Header.Set("Content-Type", "application/json")
	if o.title != "" {
		req.Header.Set("X-Title", o.title)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("llm: запрос к openrouter: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("llm: openrouter вернул %d: %s",
			resp.StatusCode, truncate(string(payload), 300))
	}

	var parsed orResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return Response{}, fmt.Errorf("llm: ответ openrouter не разобрался: %w", err)
	}
	if parsed.Error != nil {
		return Response{}, fmt.Errorf("llm: openrouter: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return Response{}, fmt.Errorf("llm: openrouter не вернул ни одного варианта")
	}

	out := Response{
		Text: parsed.Choices[0].Message.Content,
		Usage: Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		},
	}
	// Стоимость от маршрутизатора авторитетнее нашей таблицы: он знает, на
	// кого фактически ушёл запрос.
	if parsed.Usage.Cost > 0 {
		out.CostMicro = int64(parsed.Usage.Cost * 1_000_000)
	}
	return out, nil
}

// Stream — потоковая генерация OpenRouter (OpenAI-совместимый SSE). Схему
// потоково не отдаём: шлюз зовёт Stream только при пустой Schema. Дельты
// приходят из choices[].delta.content, финальный usage — из чанка с полем
// usage (просим его тем же Include, что и в Complete).
func (o *openRouter) Stream(ctx context.Context, model string, r Request) (Stream, error) {
	if o.key == "" {
		return nil, fmt.Errorf("llm: OPENROUTER_API_KEY не задан")
	}
	body := orRequest{
		Model:     model,
		MaxTokens: r.MaxTokens,
		Stream:    true,
		Usage:     &orUsageOpt{Include: true},
	}
	if r.System != "" {
		body.Messages = append(body.Messages, orMessage{Role: "system", Content: r.System})
	}
	body.Messages = append(body.Messages, orMessage{Role: "user", Content: r.Input})

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if o.title != "" {
		req.Header.Set("X-Title", o.title)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: запрос к openrouter: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("llm: openrouter вернул %d: %s",
			resp.StatusCode, truncate(string(payload), 300))
	}
	return &orStream{resp: resp, br: bufio.NewReader(resp.Body)}, nil
}

// orStreamChunk — один SSE-чанк OpenRouter. Либо приращение текста в delta,
// либо финальный usage (в отдельном чанке, часто с пустым choices).
type orStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		Cost             float64 `json:"cost"`
	} `json:"usage"`
}

// orStream разбирает SSE лениво: каждый Recv читает строки до следующей дельты
// текста, [DONE] или конца тела. Финальный usage копится в final по пути.
type orStream struct {
	resp  *http.Response
	br    *bufio.Reader
	final Response
}

func (s *orStream) Recv() (Delta, error) {
	for {
		line, err := s.br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return Delta{}, io.EOF
			}
			return Delta{}, err
		}
		line = strings.TrimSpace(line)
		// Пустые строки — разделители событий; строки с ':' — комментарии/
		// keepalive OpenRouter. И то, и другое пропускаем.
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[len("data:"):])
		if data == "[DONE]" {
			return Delta{}, io.EOF
		}
		var chunk orStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Битый чанк не рвёт поток: пропускаем и читаем дальше.
			continue
		}
		if chunk.Usage != nil {
			s.final.Usage = Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			if chunk.Usage.Cost > 0 {
				s.final.CostMicro = int64(chunk.Usage.Cost * 1_000_000)
			}
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			return Delta{Text: chunk.Choices[0].Delta.Content}, nil
		}
		// Чанк без текста (роль, usage-only) — читаем следующий.
	}
}

func (s *orStream) Response() Response { return s.final }
func (s *orStream) Close() error       { return s.resp.Body.Close() }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
