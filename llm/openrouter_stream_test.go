package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseReply — записанный поток OpenRouter: два текстовых чанка, затем usage и
// [DONE]. Пустая строка разделяет события, как в SSE.
const sseReply = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"Прич\"}}]}\n\n" +
	": keepalive\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"ал в тумане\"}}]}\n\n" +
	"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":8,\"cost\":0.00042}}\n\n" +
	"data: [DONE]\n\n"

func TestOpenRouterStream(t *testing.T) {
	var gotStream bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Убедимся, что запросили именно поток.
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotStream = body.Stream
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, sseReply)
	}))
	defer srv.Close()

	p := NewOpenRouter(ORWithAPIKey("test-key"), ORWithBaseURL(srv.URL))
	sp, ok := p.(StreamProvider)
	if !ok {
		t.Fatal("openRouter не реализует StreamProvider")
	}
	st, err := sp.Stream(context.Background(), "some/model", Request{Input: "опиши причал"})
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
	if !gotStream {
		t.Error("запрос ушёл без stream:true")
	}
	if got := b.String(); got != "Причал в тумане" {
		t.Fatalf("склейка дельт %q", got)
	}
	resp := st.Response()
	if resp.Usage.InputTokens != 120 || resp.Usage.OutputTokens != 8 {
		t.Fatalf("usage из потока: %+v", resp.Usage)
	}
	if resp.CostMicro != 420 {
		t.Fatalf("стоимость из потока %d мкд, ждали 420", resp.CostMicro)
	}
}
