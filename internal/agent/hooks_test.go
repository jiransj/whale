package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/usewhale/whale/internal/core"
)

func TestHookRunnerPreToolBlockByExitCode2(t *testing.T) {
	r := NewHookRunner([]ResolvedHook{{HookConfig: HookConfig{Command: "deny"}, Event: HookEventPreToolUse}}, ".")
	r.spawner = func(_ context.Context, _ HookSpawnInput) HookSpawnResult {
		return HookSpawnResult{ExitCode: 2, Stderr: "denied"}
	}
	report := r.Run(context.Background(), HookPayload{Event: HookEventPreToolUse, ToolName: "bash"})
	if !report.Blocked {
		t.Fatal("expected blocked report")
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != HookDecisionBlock {
		t.Fatalf("unexpected outcomes: %+v", report.Outcomes)
	}
}

func TestHookRunnerPostToolWarnByExitCode2(t *testing.T) {
	r := NewHookRunner([]ResolvedHook{{HookConfig: HookConfig{Command: "post"}, Event: HookEventPostToolUse}}, ".")
	r.spawner = func(_ context.Context, _ HookSpawnInput) HookSpawnResult {
		return HookSpawnResult{ExitCode: 2, Stderr: "warn"}
	}
	report := r.Run(context.Background(), HookPayload{Event: HookEventPostToolUse, ToolName: "echo"})
	if report.Blocked {
		t.Fatal("post hook should not block on exit 2")
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != HookDecisionWarn {
		t.Fatalf("unexpected outcomes: %+v", report.Outcomes)
	}
}

type preBlockProvider struct{ calls int }

func (p *preBlockProvider) StreamResponse(_ context.Context, _ []Message, _ []Tool) <-chan ProviderEvent {
	out := make(chan ProviderEvent, 1)
	p.calls++
	if p.calls == 1 {
		out <- ProviderEvent{Type: EventComplete, Response: &ProviderResponse{FinishReason: FinishReasonToolUse, ToolCalls: []ToolCall{{ID: "tc-1", Name: "echo", Input: `{"x":1}`}}}}
		close(out)
		return out
	}
	out <- ProviderEvent{Type: EventComplete, Response: &ProviderResponse{FinishReason: FinishReasonEndTurn, Content: "done"}}
	close(out)
	return out
}

func TestAgentPreToolHookBlockSkipsDispatch(t *testing.T) {
	store := NewInMemoryStore()
	toolCalled := false
	tool := staticTool{name: "echo", run: func(_ context.Context, _ ToolCall) (ToolResult, error) {
		toolCalled = true
		return ToolResult{ToolCallID: "tc-1", Name: "echo", Content: "ok"}, nil
	}}
	a := NewAgentWithRegistry(&preBlockProvider{}, store, core.NewToolRegistry([]core.Tool{tool}), WithHooks([]ResolvedHook{{HookConfig: HookConfig{Command: "deny"}, Event: HookEventPreToolUse}}, "."))
	a.hooks.spawner = func(_ context.Context, _ HookSpawnInput) HookSpawnResult {
		return HookSpawnResult{ExitCode: 2, Stderr: "nope"}
	}
	_, err := a.Run(context.Background(), "s-pre-block", "hi")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if toolCalled {
		t.Fatal("tool should not be called when PreToolUse blocks")
	}
}

func TestLoadHooksProjectThenGlobalOrder(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	ws := filepath.Join(root, "ws")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home hooks failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".whale"), 0o755); err != nil {
		t.Fatalf("mkdir workspace hooks failed: %v", err)
	}
	projectCfg := "[[hooks.PreToolUse]]\ncommand = \"echo project\"\n"
	globalCfg := "[[hooks.PreToolUse]]\ncommand = \"echo global\"\n"
	if err := os.WriteFile(filepath.Join(ws, ".whale", "config.toml"), []byte(projectCfg), 0o600); err != nil {
		t.Fatalf("write project config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(globalCfg), 0o600); err != nil {
		t.Fatalf("write global config failed: %v", err)
	}
	hooks, loaded, err := LoadHooks(ws, home)
	if err != nil {
		t.Fatalf("load hooks failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded sources, got %d", len(loaded))
	}
	if len(hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(hooks))
	}
	if hooks[0].Command != "echo project" || hooks[1].Command != "echo global" {
		t.Fatalf("unexpected order: %+v", hooks)
	}
}

func TestHookRunnerBlockShortCircuitsFollowingHooks(t *testing.T) {
	hooks := []ResolvedHook{
		{HookConfig: HookConfig{Command: "first"}, Event: HookEventPreToolUse},
		{HookConfig: HookConfig{Command: "second"}, Event: HookEventPreToolUse},
	}
	r := NewHookRunner(hooks, ".")
	calls := 0
	r.spawner = func(_ context.Context, in HookSpawnInput) HookSpawnResult {
		calls++
		if in.Command == "first" {
			return HookSpawnResult{ExitCode: 2, Stderr: "blocked"}
		}
		return HookSpawnResult{ExitCode: 0}
	}
	report := r.Run(context.Background(), HookPayload{Event: HookEventPreToolUse, ToolName: "bash"})
	if !report.Blocked {
		t.Fatal("expected blocked")
	}
	if calls != 1 {
		t.Fatalf("expected short-circuit after first hook, calls=%d", calls)
	}
}

func TestAgentDoesNotTriggerUserPromptOrStopHooks(t *testing.T) {
	store := NewInMemoryStore()
	provider := &noToolProvider{}
	hooks := []ResolvedHook{
		{HookConfig: HookConfig{Command: "exit 2"}, Event: HookEventUserPromptSubmit},
		{HookConfig: HookConfig{Command: "exit 2"}, Event: HookEventStop},
	}
	a := NewAgentWithRegistry(provider, store, core.NewToolRegistry(nil), WithHooks(hooks, "."))
	calls := 0
	a.hooks.spawner = func(_ context.Context, _ HookSpawnInput) HookSpawnResult {
		calls++
		return HookSpawnResult{ExitCode: 2}
	}
	_, err := a.Run(context.Background(), "s-no-app-hooks", "hello")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected 0 hook invocations in agent for UserPromptSubmit/Stop, got %d", calls)
	}
}

type noToolProvider struct{}

func (p *noToolProvider) StreamResponse(_ context.Context, _ []Message, _ []Tool) <-chan ProviderEvent {
	out := make(chan ProviderEvent, 1)
	out <- ProviderEvent{Type: EventComplete, Response: &ProviderResponse{FinishReason: FinishReasonEndTurn, Content: "ok"}}
	close(out)
	return out
}

func TestHookRunnerRealShellPreToolBlock(t *testing.T) {
	r := NewHookRunner([]ResolvedHook{{HookConfig: HookConfig{Command: "echo blocked >&2; exit 2"}, Event: HookEventPreToolUse}}, ".")
	report := r.Run(context.Background(), HookPayload{Event: HookEventPreToolUse, ToolName: "bash"})
	if !report.Blocked {
		t.Fatal("expected blocked")
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != HookDecisionBlock {
		t.Fatalf("unexpected outcomes: %+v", report.Outcomes)
	}
}

func TestHookRunnerRealShellPostToolWarn(t *testing.T) {
	r := NewHookRunner([]ResolvedHook{{HookConfig: HookConfig{Command: "echo post-warn >&2; exit 5"}, Event: HookEventPostToolUse}}, ".")
	report := r.Run(context.Background(), HookPayload{Event: HookEventPostToolUse, ToolName: "echo"})
	if report.Blocked {
		t.Fatal("post tool should not block")
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Decision != HookDecisionWarn {
		t.Fatalf("unexpected outcomes: %+v", report.Outcomes)
	}
}

func TestHookRunnerStopPayloadCarriesAssistantTextAndTurn(t *testing.T) {
	tmp := t.TempDir()
	capture := filepath.Join(tmp, "payload.json")
	cmd := "cat > " + capture + "; exit 0"
	r := NewHookRunner([]ResolvedHook{{HookConfig: HookConfig{Command: cmd}, Event: HookEventStop}}, ".")
	payload := NewStopPayload("s1", tmp, "final answer", 3)
	report := r.Run(context.Background(), payload)
	if report.Blocked {
		t.Fatal("stop should not block")
	}
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read payload failed: %v", err)
	}
	var got HookPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if got.Event != HookEventStop || got.LastAssistantText != "final answer" || got.Turn != 3 || got.SessionID != "s1" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

type staticTool struct {
	name string
	run  func(context.Context, ToolCall) (ToolResult, error)
}

func (t staticTool) Name() string { return t.name }
func (t staticTool) Run(ctx context.Context, call ToolCall) (ToolResult, error) {
	return t.run(ctx, call)
}
