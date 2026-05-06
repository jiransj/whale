package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/llm"
	"github.com/usewhale/whale/internal/store"
)

type usageLogProvider struct{}

func (p *usageLogProvider) StreamResponse(_ context.Context, _ []core.Message, _ []core.Tool) <-chan llm.ProviderEvent {
	out := make(chan llm.ProviderEvent, 1)
	out <- llm.ProviderEvent{
		Type: llm.EventComplete,
		Response: &llm.ProviderResponse{
			Content:      "ok",
			Usage:        llm.Usage{PromptTokens: 100, CompletionTokens: 50, PromptCacheHitTokens: 20, PromptCacheMissTokens: 80},
			Model:        "deepseek-v4-flash",
			FinishReason: core.FinishReasonEndTurn,
		},
	}
	close(out)
	return out
}

func TestRecordTurnCostWritesUsageLogWithoutSessionRuntime(t *testing.T) {
	tmp := t.TempDir()
	usagePath := filepath.Join(tmp, "usage.jsonl")
	provider := &usageLogProvider{}

	a := NewAgentWithRegistry(provider, store.NewInMemoryStore(), nil, WithUsageLogPath(usagePath))
	if _, err := a.Run(context.Background(), "usage-log-no-runtime", "hi"); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if _, err := os.Stat(usagePath); err != nil {
		t.Fatalf("usage log missing: %v", err)
	}
}
