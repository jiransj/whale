package tools

import (
	"context"
	"os"
	"path/filepath"

	"github.com/usewhale/whale/internal/core"
)

func (b *Toolset) writeFile(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	var in struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := decodeInput(call.Input, &in); err != nil {
		return marshalToolError(call, "invalid_args", err.Error()), nil
	}
	if in.FilePath == "" {
		return marshalToolError(call, "invalid_args", "file_path is required"), nil
	}
	abs, err := b.safePath(in.FilePath)
	if err != nil {
		return marshalToolError(call, "permission_denied", err.Error()), nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return marshalToolError(call, "write_failed", err.Error()), nil
	}
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		return marshalToolError(call, "write_failed", err.Error()), nil
	}
	return marshalToolResult(call, map[string]any{"file_path": in.FilePath, "bytes": len(in.Content)})
}
