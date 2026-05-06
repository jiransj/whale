package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/session"
)

type todoProvider struct{ calls int }

func (p *todoProvider) StreamResponse(_ context.Context, _ []Message, _ []Tool) <-chan ProviderEvent {
	out := make(chan ProviderEvent, 1)
	p.calls++
	if p.calls == 1 {
		out <- ProviderEvent{Type: EventComplete, Response: &ProviderResponse{FinishReason: FinishReasonToolUse, ToolCalls: []ToolCall{{ID: "t1", Name: "todo_add", Input: `{"text":"ship tools","priority":3}`}}}}
	} else if p.calls == 2 {
		out <- ProviderEvent{Type: EventComplete, Response: &ProviderResponse{FinishReason: FinishReasonToolUse, ToolCalls: []ToolCall{{ID: "t2", Name: "todo_list", Input: `{}`}}}}
	} else {
		out <- ProviderEvent{Type: EventComplete, Response: &ProviderResponse{FinishReason: FinishReasonEndTurn, Content: "done"}}
	}
	close(out)
	return out
}

func TestTodoToolsPersistInSession(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	a := NewAgentWithRegistry(
		&todoProvider{},
		NewInMemoryStore(),
		NewToolRegistry(nil),
		WithSessionsDir(sessionsDir),
	)
	if _, err := a.Run(context.Background(), "s-todo", "go"); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	st, err := session.LoadTodoState(sessionsDir, "s-todo")
	if err != nil {
		t.Fatalf("load todo: %v", err)
	}
	if len(st.Items) != 1 || !strings.Contains(st.Items[0].Text, "ship tools") {
		t.Fatalf("unexpected todo state: %+v", st)
	}
}
