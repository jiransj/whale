package compact

import (
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/core"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("abcd"); got != 1 {
		t.Fatalf("expected 1 token, got %d", got)
	}
	if got := EstimateTokens("你好"); got != 2 {
		t.Fatalf("expected 2 tokens, got %d", got)
	}
	if got := EstimateTokens("   "); got != 0 {
		t.Fatalf("expected blank text to estimate to 0, got %d", got)
	}
}

func TestEstimateMessagesTokensIncludesToolPayloads(t *testing.T) {
	msgs := []core.Message{{
		Role: core.RoleAssistant,
		Text: strings.Repeat("a", 8),
		ToolCalls: []core.ToolCall{{
			Name:  "write",
			Input: strings.Repeat("b", 8),
		}},
	}, {
		Role: core.RoleTool,
		ToolResults: []core.ToolResult{{
			Name:    "write",
			Content: strings.Repeat("c", 8),
		}},
	}}
	if got := EstimateMessagesTokens(msgs); got == 0 {
		t.Fatal("expected non-zero estimate")
	}
}
