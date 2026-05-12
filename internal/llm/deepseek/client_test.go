package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/llm"
)

type fakeTool struct{ n string }

func (f fakeTool) Name() string { return f.n }
func (f fakeTool) Run(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: "ok"}, nil
}

type nestedTool struct{}

func (nestedTool) Name() string { return "nested" }
func (nestedTool) Run(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: "ok"}, nil
}
func (nestedTool) Description() string { return "nested test tool" }
func (nestedTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"payload": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string"},
						},
						"required": []string{"path"},
					},
				},
				"required": []string{"file"},
			},
		},
		"required": []string{"payload"},
	}
}

func TestStreamResponseParsesToolCallAndContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		streamOptions, ok := payload["stream_options"].(map[string]any)
		if !ok || streamOptions["include_usage"] != true {
			t.Fatalf("expected stream_options.include_usage=true, got %#v", payload["stream_options"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello \"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"echo\",\"arguments\":\"{\"}}]}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"x\\\":1}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	_ = os.Setenv("DEEPSEEK_API_KEY", "test-key")
	c, err := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	events := c.StreamResponse(context.Background(), []core.Message{{Role: core.RoleUser, Text: "hi"}}, []core.Tool{fakeTool{"echo"}})

	var gotComplete bool
	var gotToolArgsReady bool
	for ev := range events {
		if ev.Type == llm.EventError {
			t.Fatalf("provider error: %v", ev.Err)
		}
		if ev.Type == llm.EventToolArgsDelta && ev.ToolArgsDelta != nil && ev.ToolArgsDelta.ReadyCount >= 1 {
			gotToolArgsReady = true
		}
		if ev.Type == llm.EventComplete {
			gotComplete = true
			if ev.Response == nil {
				t.Fatal("expected response")
			}
			if ev.Response.FinishReason != core.FinishReasonToolUse {
				t.Fatalf("finish reason: %s", ev.Response.FinishReason)
			}
			if len(ev.Response.ToolCalls) != 1 {
				t.Fatalf("tool calls: %d", len(ev.Response.ToolCalls))
			}
			if ev.Response.ToolCalls[0].Name != "echo" {
				t.Fatalf("tool name: %s", ev.Response.ToolCalls[0].Name)
			}
		}
	}
	if !gotComplete {
		t.Fatal("missing complete event")
	}
	if !gotToolArgsReady {
		t.Fatal("missing tool args ready progress event")
	}
}

func TestStreamResponseParsesReasoningDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking...\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	_ = os.Setenv("DEEPSEEK_API_KEY", "test-key")
	c, err := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	events := c.StreamResponse(context.Background(), []core.Message{{Role: core.RoleUser, Text: "hi"}}, nil)
	var sawReasoning bool
	var sawDone bool
	for ev := range events {
		if ev.Type == llm.EventError {
			t.Fatalf("provider error: %v", ev.Err)
		}
		if ev.Type == llm.EventReasoningDelta && ev.ReasoningDelta == "thinking..." {
			sawReasoning = true
		}
		if ev.Type == llm.EventComplete && ev.Response != nil && ev.Response.Reasoning == "thinking..." {
			sawDone = true
		}
	}
	if !sawReasoning {
		t.Fatal("expected reasoning delta event")
	}
	if !sawDone {
		t.Fatal("expected complete response with reasoning")
	}
}

func TestToDeepSeekMessagesRecoversMissingToolResults(t *testing.T) {
	history := []core.Message{
		{
			Role: core.RoleAssistant,
			ToolCalls: []core.ToolCall{
				{ID: "call_1", Name: "echo", Input: `{"x":1}`},
			},
		},
		{Role: core.RoleUser, Text: "next"},
	}
	out := toDeepSeekMessages(history)
	if len(out) < 3 {
		t.Fatalf("expected recovered tool message inserted, got %d", len(out))
	}
	if out[1]["role"] != "tool" || out[1]["tool_call_id"] != "call_1" {
		t.Fatalf("expected recovered tool response for call_1, got %+v", out[1])
	}
}

func TestToDeepSeekMessagesDropsStrayToolResults(t *testing.T) {
	history := []core.Message{
		{Role: core.RoleUser, Text: "hi"},
		{
			Role: core.RoleTool,
			ToolResults: []core.ToolResult{
				{ToolCallID: "orphan", Name: "bash", Content: "x"},
			},
		},
	}
	out := toDeepSeekMessages(history)
	if len(out) != 1 {
		t.Fatalf("expected stray tool message dropped, got %d", len(out))
	}
	if out[0]["role"] != "user" {
		t.Fatalf("unexpected first role: %+v", out[0])
	}
}

func TestToDeepSeekMessagesDoesNotSendToolResultMetadata(t *testing.T) {
	history := []core.Message{
		{
			Role: core.RoleAssistant,
			ToolCalls: []core.ToolCall{
				{ID: "call_1", Name: "edit", Input: `{"file_path":"a.txt"}`},
			},
		},
		{
			Role: core.RoleTool,
			ToolResults: []core.ToolResult{
				{
					ToolCallID: "call_1",
					Name:       "edit",
					Content:    `{"success":true}`,
					Metadata: map[string]any{
						"kind":  "file_diff",
						"files": []any{map[string]any{"unified_diff": "-old\n+new"}},
					},
				},
			},
		},
	}
	out := toDeepSeekMessages(history)
	if len(out) != 2 {
		t.Fatalf("expected assistant and tool messages, got %d", len(out))
	}
	toolMsg := out[1]
	if _, ok := toolMsg["metadata"]; ok {
		t.Fatalf("metadata must not be sent to provider: %+v", toolMsg)
	}
	if content, _ := toolMsg["content"].(string); strings.Contains(content, "unified_diff") || strings.Contains(content, "-old") {
		t.Fatalf("tool content must not include metadata diff: %q", content)
	}
}

func TestToDeepSeekTools_FlattensDeepSchema(t *testing.T) {
	out := toDeepSeekTools([]core.Tool{nestedTool{}})
	if len(out) != 1 {
		t.Fatalf("expected one tool, got %d", len(out))
	}
	fn, _ := out[0]["function"].(map[string]any)
	params, _ := fn["parameters"].(map[string]any)
	props, _ := params["properties"].(map[string]any)
	if _, ok := props["payload.file.path"]; !ok {
		t.Fatalf("expected flattened payload.file.path in properties: %#v", props)
	}
}

func TestEstimateReasoningReplayTokens(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "reasoning_content": "12345678"},
		{"role": "assistant", "reasoning_content": "1234"},
	}
	got := estimateReasoningReplayTokens(msgs)
	if got != 3 {
		t.Fatalf("expected replay tokens=3, got %d", got)
	}
}
