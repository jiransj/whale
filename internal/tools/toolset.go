package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/usewhale/whale/internal/core"
)

type Toolset struct {
	root          string
	httpClient    *http.Client
	ddgSearchURL  string
	bingSearchURL string
	tasks         *shellTaskRegistry
}

func NewToolset(root string) (*Toolset, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	return &Toolset{
		root:          abs,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		ddgSearchURL:  "https://html.duckduckgo.com/html/?q=%s",
		bingSearchURL: "https://www.bing.com/search?q=%s",
		tasks:         newShellTaskRegistry(),
	}, nil
}

func marshalToolResult(call core.ToolCall, data any) (core.ToolResult, error) {
	return marshalToolResultWithMetadata(call, data, nil)
}

func marshalToolResultWithMetadata(call core.ToolCall, data any, metadata map[string]any) (core.ToolResult, error) {
	dataMap, ok := data.(map[string]any)
	if !ok {
		dataMap = map[string]any{"payload": data}
	}
	content, err := core.MarshalToolEnvelope(core.NewToolSuccessEnvelope(dataMap))
	if err != nil {
		return core.ToolResult{}, err
	}
	return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: content, Metadata: metadata}, nil
}

func marshalToolError(call core.ToolCall, code, msg string) core.ToolResult {
	content, err := core.MarshalToolEnvelope(core.NewToolErrorEnvelope(code, msg))
	if err != nil {
		content = fmt.Sprintf(`{"success":false,"code":%q,"message":%q}`, code, msg)
	}
	return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: content, IsError: true}
}

func (b *Toolset) safePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "."
	}
	for strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "\\") {
		raw = raw[1:]
	}
	target := filepath.Clean(filepath.Join(b.root, raw))
	rel, err := filepath.Rel(b.root, target)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes workspace: %s", raw)
	}
	return target, nil
}

func decodeInput(raw string, out any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}
