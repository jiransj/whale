package mcp

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usewhale/whale/internal/core"
)

func TestToolParametersCoerceSchema(t *testing.T) {
	tool := &Tool{registeredName: "mcp__fs__read", spec: &sdk.Tool{Name: "read"}}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Fatalf("params: %+v", params)
	}
}

func TestMCPResultWrapsTextAndMedia(t *testing.T) {
	res := mcpResult(core.ToolCall{ID: "call", Name: "mcp__img__show"}, "img", "show", &sdk.CallToolResult{
		Content: []sdk.Content{
			&sdk.TextContent{Text: "hello"},
			&sdk.ImageContent{MIMEType: "image/png", Data: []byte("abc")},
		},
	})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	if !strings.Contains(res.Content, "hello") || !strings.Contains(res.Content, "image/png") {
		t.Fatalf("content: %s", res.Content)
	}
}

func TestMCPResultMarksToolError(t *testing.T) {
	res := mcpResult(core.ToolCall{ID: "call", Name: "mcp__fs__read"}, "fs", "read", &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: "failed"}},
		IsError: true,
	})
	if !res.IsError || !strings.Contains(res.Content, "mcp_tool_error") {
		t.Fatalf("result: %+v", res)
	}
}

func TestToolRunRejectsInvalidJSON(t *testing.T) {
	tool := &Tool{registeredName: "mcp__fs__read", serverName: "fs", toolName: "read"}
	res, err := tool.Run(context.Background(), core.ToolCall{ID: "call", Name: tool.Name(), Input: "not-json"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Content, "invalid_mcp_input") {
		t.Fatalf("result: %+v", res)
	}
}
