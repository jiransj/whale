package policy

import (
	"testing"

	"github.com/usewhale/whale/internal/core"
)

func TestDefaultToolPolicyRequiresApprovalForMCPTools(t *testing.T) {
	decision := DefaultToolPolicy{Mode: ApprovalModeOnRequest}.Decide(
		core.ToolSpec{Name: "mcp__github__create_issue"},
		core.ToolCall{Name: "mcp__github__create_issue", Input: `{}`},
	)
	if !decision.Allow || !decision.RequiresApproval {
		t.Fatalf("decision: %+v", decision)
	}
}

func TestDefaultToolPolicyAllowsReadOnlyMCPTools(t *testing.T) {
	decision := DefaultToolPolicy{Mode: ApprovalModeOnRequest}.Decide(
		core.ToolSpec{Name: "mcp__fs__read", ReadOnly: true},
		core.ToolCall{Name: "mcp__fs__read", Input: `{}`},
	)
	if !decision.Allow || decision.RequiresApproval || decision.Code != "read_only" {
		t.Fatalf("decision: %+v", decision)
	}
}
