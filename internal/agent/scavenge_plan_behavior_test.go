package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/session"
)

type scavengingProvider struct{}

func (p *scavengingProvider) StreamResponse(_ context.Context, _ []Message, _ []Tool) <-chan ProviderEvent {
	return eventStream(
		ProviderEvent{
			Type:           EventReasoningDelta,
			ReasoningDelta: `need read {"name":"read_file","arguments":{"file_path":"x.txt"}}`,
		},
		endTurnEvent("done"),
	)
}

func TestScavengeFromReasoningWhenNoDeclaredToolCalls(t *testing.T) {
	store := NewInMemoryStore()
	a := NewAgentWithRegistry(
		&scavengingProvider{},
		store,
		NewToolRegistry([]Tool{viewLikeTool{}}),
	)

	events, err := a.RunStream(context.Background(), "s-scavenge", "go")
	if err != nil {
		t.Fatalf("run stream failed: %v", err)
	}
	var sawScavenge bool
	for ev := range events {
		if ev.Type == AgentEventTypeToolCallScavenged && ev.Scavenged != nil && ev.Scavenged.Count == 1 {
			sawScavenge = true
		}
	}
	if !sawScavenge {
		t.Fatal("expected tool_call_scavenged event")
	}
	msgs, _ := store.List(context.Background(), "s-scavenge")
	if len(msgs) < 3 {
		t.Fatalf("expected tool message generated, got %d messages", len(msgs))
	}
	if msgs[2].Role != RoleTool || len(msgs[2].ToolResults) == 0 {
		t.Fatalf("expected tool results from scavenged call, got: %+v", msgs[2])
	}
}

func TestPlanModeBlocksWriteTools(t *testing.T) {
	store := NewInMemoryStore()
	prov := &approvalProvider{}
	a := NewAgentWithRegistry(
		prov,
		store,
		NewToolRegistry([]Tool{writeLikeTool{}}),
		WithSessionMode(session.ModePlan),
	)
	events, err := a.RunStream(context.Background(), "s-plan-block", "go")
	if err != nil {
		t.Fatalf("run stream failed: %v", err)
	}
	var sawModeBlocked bool
	var sawPlanBlockedResult bool
	for ev := range events {
		if ev.Type == AgentEventTypeToolModeBlocked && ev.ToolBlocked != nil && ev.ToolBlocked.ReasonCode == "plan_mode_blocked" {
			sawModeBlocked = true
		}
		if ev.Type == AgentEventTypeToolResult && ev.Result != nil && strings.Contains(ev.Result.Content, "plan_mode_blocked") {
			sawPlanBlockedResult = true
		}
	}
	if !sawModeBlocked {
		t.Fatal("expected tool mode blocked event")
	}
	if !sawPlanBlockedResult {
		t.Fatal("expected plan mode blocked tool result")
	}
}
