package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicSSE — записанный поток Anthropic: usage входа в message_start, два
// текстовых приращения, usage выхода в message_delta.
const anthropicSSE = "event: message_start\n" +
	"data: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"role\":\"assistant\",\"usage\":{\"input_tokens\":15,\"output_tokens\":1}}}\n\n" +
	"event: content_block_start\n" +
	"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
	"event: content_block_delta\n" +
	"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Прич\"}}\n\n" +
	"event: content_block_delta\n" +
	"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ал\"}}\n\n" +
	"event: content_block_stop\n" +
	"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
	"event: message_delta\n" +
	"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":6}}\n\n" +
	"event: message_stop\n" +
	"data: {\"type\":\"message_stop\"}\n\n"

func TestAnthropicStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, anthropicSSE)
	}))
	defer srv.Close()

	p, ok := NewAnthropic(option.WithBaseURL(srv.URL), option.WithAPIKey("test")).(StreamProvider)
	if !ok {
		t.Fatal("anthropicProvider не реализует StreamProvider")
	}
	st, err := p.Stream(context.Background(), "claude-sonnet-5", Request{Input: "опиши причал"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer st.Close()

	var b strings.Builder
	for {
		d, err := st.Recv()
		b.WriteString(d.Text)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
	}
	if got := b.String(); got != "Причал" {
		t.Fatalf("склейка дельт %q, ждали Причал", got)
	}
	resp := st.Response()
	if resp.Usage.InputTokens != 15 || resp.Usage.OutputTokens != 6 {
		t.Fatalf("usage из потока: %+v", resp.Usage)
	}
}
