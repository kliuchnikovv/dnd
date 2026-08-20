package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// orServer поднимает подставной OpenRouter и отдаёт записанный запрос.
func orServer(t *testing.T, status int, reply string) (*httptest.Server, *orRequest, *http.Header) {
	t.Helper()
	var got orRequest
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		if r.URL.Path != "/chat/completions" {
			t.Errorf("путь %q, ожидался /chat/completions", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("тело запроса не разобралось: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &hdr
}

func orProvider(t *testing.T, srv *httptest.Server, opts ...OROption) Provider {
	t.Helper()
	opts = append([]OROption{ORWithAPIKey("test-key"), ORWithBaseURL(srv.URL)}, opts...)
	return NewOpenRouter(opts...)
}

const orReply = `{"choices":[{"message":{"role":"assistant","content":"{\"outcome\":\"clarify\"}"},
 "finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":8,"cost":0.00042}}`

func TestOpenRouterSendsExpectedRequest(t *testing.T) {
	srv, got, hdr := orServer(t, 200, orReply)
	p := orProvider(t, srv)

	resp, err := p.Complete(context.Background(), "anthropic/claude-sonnet-4.5", Request{
		Role: RoleIntentParser, System: "инструкция", Input: "смотрю по сторонам",
		MaxTokens: 512, Schema: `{"type":"object","properties":{"outcome":{"type":"string"}},"required":["outcome"]}`,
	})
	if err != nil {
		t.Fatalf("вызов: %v", err)
	}

	if hdr.Get("Authorization") != "Bearer test-key" {
		t.Errorf("заголовок авторизации: %q", hdr.Get("Authorization"))
	}
	if got.Model != "anthropic/claude-sonnet-4.5" {
		t.Errorf("модель %q", got.Model)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[1].Role != "user" {
		t.Errorf("сообщения: %+v", got.Messages)
	}
	if got.MaxTokens != 512 {
		t.Errorf("max_tokens %d", got.MaxTokens)
	}
	if got.ResponseFormat == nil {
		t.Fatal("response_format не отправлен — схема не запрошена")
	}
	if got.ResponseFormat.Type != "json_schema" || !got.ResponseFormat.JSONSchema.Strict {
		t.Errorf("response_format: %+v", got.ResponseFormat)
	}
	if got.Usage == nil || !got.Usage.Include {
		t.Error("не запрошен учёт стоимости — таблица цен для маршрутизатора врёт")
	}
	if resp.Text != `{"outcome":"clarify"}` {
		t.Errorf("текст ответа %q", resp.Text)
	}
	if resp.Usage.InputTokens != 120 || resp.Usage.OutputTokens != 8 {
		t.Errorf("расход токенов: %+v", resp.Usage)
	}
	// 0.00042 $ = 420 микродолларов
	if resp.CostMicro != 420 {
		t.Errorf("стоимость %d мкд, ожидалось 420", resp.CostMicro)
	}
}

func TestOpenRouterOmitsSchemaWhenNotRequested(t *testing.T) {
	srv, got, _ := orServer(t, 200, orReply)
	if _, err := orProvider(t, srv).Complete(context.Background(), "m", Request{Input: "проза"}); err != nil {
		t.Fatal(err)
	}
	if got.ResponseFormat != nil {
		t.Error("схема отправлена там, где её не просили")
	}
	if len(got.Messages) != 1 {
		t.Errorf("сообщений %d, ожидалось 1 без системного", len(got.Messages))
	}
}

func TestOpenRouterSurfacesHTTPError(t *testing.T) {
	srv, _, _ := orServer(t, 429, `{"error":{"message":"rate limited"}}`)
	_, err := orProvider(t, srv).Complete(context.Background(), "m", Request{Input: "x"})
	if err == nil {
		t.Fatal("429 не дал ошибки")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("ошибка не называет код: %v", err)
	}
}

func TestOpenRouterSurfacesBodyError(t *testing.T) {
	srv, _, _ := orServer(t, 200, `{"error":{"message":"no endpoints found"}}`)
	_, err := orProvider(t, srv).Complete(context.Background(), "m", Request{Input: "x"})
	if err == nil || !strings.Contains(err.Error(), "no endpoints found") {
		t.Fatalf("ошибка в теле при 200 не поднята: %v", err)
	}
}

func TestOpenRouterRequiresKey(t *testing.T) {
	srv, _, _ := orServer(t, 200, orReply)
	p := NewOpenRouter(ORWithAPIKey(""), ORWithBaseURL(srv.URL))
	if _, err := p.Complete(context.Background(), "m", Request{Input: "x"}); err == nil {
		t.Fatal("вызов без ключа прошёл")
	}
}

func TestOpenRouterStrictnessIsDeclarable(t *testing.T) {
	if !NewOpenRouter(ORWithAPIKey("k")).StrictOutput() {
		t.Error("по умолчанию ожидалась заявленная строгость")
	}
	if NewOpenRouter(ORWithAPIKey("k"), ORWithStrictOutput(false)).StrictOutput() {
		t.Error("ORWithStrictOutput(false) не применился")
	}
	// Заявив нестрогость, провайдер сам себя отсекает от парсера.
	loose := NewOpenRouter(ORWithAPIKey("k"), ORWithStrictOutput(false))
	r := NewRouter().Route(RoleIntentParser, Target{Provider: loose, Model: "m"})
	if len(r.Chain(RoleIntentParser)) != 0 {
		t.Error("нестрогий провайдер допущен к роли, мутирующей состояние")
	}
}

// Стоимость от маршрутизатора должна пройти через шлюз как есть: его модели
// в нашей таблице цен отсутствуют по построению.
func TestGatewayTrustsProviderReportedCost(t *testing.T) {
	srv, _, _ := orServer(t, 200, orReply)
	p := orProvider(t, srv)
	g := NewGateway(NewRouter().Route(RoleNarrator,
		Target{Provider: p, Model: "anthropic/claude-sonnet-4.5"}),
		NewLedger(Caps{}, WithClock(fixedClock("2026-08-18"))))

	resp, err := g.Do(context.Background(), Request{Role: RoleNarrator, Input: "сцена"})
	if err != nil {
		t.Fatalf("модель вне таблицы цен отвергнута, хотя стоимость известна: %v", err)
	}
	if resp.CostMicro != 420 {
		t.Errorf("стоимость %d, ожидалось 420", resp.CostMicro)
	}
	if s := g.Stats(); s.SpentMicro != 420 {
		t.Errorf("учтено %d мкд", s.SpentMicro)
	}
}
