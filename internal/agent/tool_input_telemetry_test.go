package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/policy"
	"github.com/usewhale/whale/internal/telemetry"
)

type telemetryToolProvider struct {
	calls int
	tool  string
	input string
}

func (p *telemetryToolProvider) StreamResponse(_ context.Context, _ []Message, _ []Tool) <-chan ProviderEvent {
	p.calls++
	if p.calls == 1 {
		ev := toolUseEvent(toolCall("tc-telemetry", p.tool, p.input))
		ev.Response.Model = "deepseek-v4-pro"
		return eventStream(ev)
	}
	ev := endTurnEvent("done")
	ev.Response.Model = "deepseek-v4-pro"
	return eventStream(ev)
}

type telemetryNestedTool struct{}

func (t telemetryNestedTool) Name() string { return "nested" }

func (t telemetryNestedTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"payload": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
		"additionalProperties": true,
	}
}

func (t telemetryNestedTool) Run(_ context.Context, call ToolCall) (ToolResult, error) {
	return ToolResult{ToolCallID: call.ID, Name: call.Name, Content: `{"success":true,"code":"ok"}`}, nil
}

type requiredPathTool struct{}

func (t requiredPathTool) Name() string { return "needs_path" }

func (t requiredPathTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string"},
		},
		"required":             []string{"file_path"},
		"additionalProperties": true,
	}
}

func (t requiredPathTool) Run(_ context.Context, call ToolCall) (ToolResult, error) {
	return ToolResult{ToolCallID: call.ID, Name: call.Name, Content: `{"success":true,"code":"ok"}`}, nil
}

type decodeArgsTool struct{}

func (t decodeArgsTool) Name() string { return "decode_args" }

func (t decodeArgsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
		"additionalProperties": true,
	}
}

func (t decodeArgsTool) Run(_ context.Context, call ToolCall) (ToolResult, error) {
	var in struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(call.Input), &in); err != nil {
		return ToolResult{ToolCallID: call.ID, Name: call.Name, Content: `{"success":false,"error":"bad count","code":"invalid_args"}`, IsError: true}, nil
	}
	return ToolResult{ToolCallID: call.ID, Name: call.Name, Content: `{"success":true,"code":"ok"}`}, nil
}

func TestToolInputTelemetryRecordsTruncatedJSONRepair(t *testing.T) {
	dir := t.TempDir()
	a := NewAgentWithRegistry(
		&telemetryToolProvider{tool: "write", input: `{"file_path":"a.txt","content":"x"`},
		NewInMemoryStore(),
		NewToolRegistry([]Tool{writeLikeTool{}}),
		WithSessionsDir(dir),
		WithToolPolicy(policyNever()),
	)
	drainAgentEvents(t, a, "s-truncated")

	events := readToolInputEvents(t, dir, "s-truncated")
	assertToolInputEvent(t, events, "tool_input_repaired", "truncated_json", "")
}

func TestToolInputTelemetryRecordsRenestFlatInputRepair(t *testing.T) {
	dir := t.TempDir()
	a := NewAgentWithRegistry(
		&telemetryToolProvider{tool: "nested", input: `{"payload.target.path":"a.txt"}`},
		NewInMemoryStore(),
		NewToolRegistry([]Tool{telemetryNestedTool{}}),
		WithSessionsDir(dir),
		WithToolPolicy(policyNever()),
	)
	drainAgentEvents(t, a, "s-renest")

	events := readToolInputEvents(t, dir, "s-renest")
	assertToolInputEvent(t, events, "tool_input_repaired", "renest_flat_input", "")
}

func TestToolInputTelemetryRecordsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	a := NewAgentWithRegistry(
		&telemetryToolProvider{tool: "needs_path", input: `{}`},
		NewInMemoryStore(),
		NewToolRegistry([]Tool{requiredPathTool{}}),
		WithSessionsDir(dir),
		WithToolPolicy(policyNever()),
		WithRecoveryPolicy(RecoveryPolicy{Enabled: false}),
	)
	drainAgentEvents(t, a, "s-invalid-input")

	events := readToolInputEvents(t, dir, "s-invalid-input")
	assertToolInputEvent(t, events, "tool_input_invalid", "", "invalid_input")
}

func TestToolInputTelemetryRecordsInvalidArgs(t *testing.T) {
	dir := t.TempDir()
	a := NewAgentWithRegistry(
		&telemetryToolProvider{tool: "decode_args", input: `{"count":"bad"}`},
		NewInMemoryStore(),
		NewToolRegistry([]Tool{decodeArgsTool{}}),
		WithSessionsDir(dir),
		WithToolPolicy(policyNever()),
		WithRecoveryPolicy(RecoveryPolicy{Enabled: false}),
	)
	drainAgentEvents(t, a, "s-invalid-args")

	events := readToolInputEvents(t, dir, "s-invalid-args")
	assertToolInputEvent(t, events, "tool_input_invalid", "", "invalid_args")
}

func drainAgentEvents(t *testing.T, a *Agent, sessionID string) {
	t.Helper()
	ch, err := a.RunStream(context.Background(), sessionID, "go")
	if err != nil {
		t.Fatalf("run stream: %v", err)
	}
	for ev := range ch {
		if ev.Type == AgentEventTypeError {
			t.Fatalf("agent error: %v", ev.Err)
		}
	}
}

func readToolInputEvents(t *testing.T, sessionsDir, sessionID string) []telemetry.ToolInputEvent {
	t.Helper()
	b, err := os.ReadFile(telemetry.ToolInputEventsPath(sessionsDir, sessionID))
	if err != nil {
		t.Fatalf("read tool input events: %v", err)
	}
	lines := splitNonEmptyLines(string(b))
	events := make([]telemetry.ToolInputEvent, 0, len(lines))
	for _, line := range lines {
		var ev telemetry.ToolInputEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("unmarshal event %q: %v", line, err)
		}
		events = append(events, ev)
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Fatalf("unmarshal raw event %q: %v", line, err)
		}
		if _, ok := raw["input"]; ok {
			t.Fatalf("tool input event must not contain raw input: %v", raw)
		}
	}
	return events
}

func assertToolInputEvent(t *testing.T, events []telemetry.ToolInputEvent, event, repairKind, errorCode string) {
	t.Helper()
	for _, ev := range events {
		if ev.Event == event && ev.RepairKind == repairKind && ev.ErrorCode == errorCode {
			if ev.Session == "" || ev.ToolCallID == "" || ev.Tool == "" || ev.AssistantMessageID == "" {
				t.Fatalf("event missing identity fields: %+v", ev)
			}
			return
		}
	}
	t.Fatalf("missing event=%s repair=%s error=%s in %+v", event, repairKind, errorCode, events)
}

func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func policyNever() policy.ToolPolicy {
	return policy.DefaultToolPolicy{Mode: policy.ApprovalModeNever}
}

func TestToolInputEventsUseSessionSidecarName(t *testing.T) {
	dir := t.TempDir()
	path := telemetry.ToolInputEventsPath(dir, "s1")
	if filepath.Base(path) != "s1.tool_input_events.jsonl" {
		t.Fatalf("unexpected sidecar path: %s", path)
	}
}
