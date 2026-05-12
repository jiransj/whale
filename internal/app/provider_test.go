package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/llm"
)

func TestNewDeepSeekProviderAppliesBaseURL(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "test-key")
	t.Setenv("DEEPSEEK_BASE_URL", "https://env.example")
	var sawRequest bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	provider, err := newDeepSeekProvider(providerOptions{
		BaseURL:         srv.URL,
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "high",
		ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatalf("newDeepSeekProvider: %v", err)
	}
	for ev := range provider.StreamResponse(context.Background(), []core.Message{{Role: core.RoleUser, Text: "hi"}}, nil) {
		if ev.Type == llm.EventError {
			t.Fatalf("provider error: %v", ev.Err)
		}
	}
	if !sawRequest {
		t.Fatal("expected request to configured base URL")
	}
}
