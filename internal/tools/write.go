package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"

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
	beforeBytes, readErr := os.ReadFile(abs)
	if readErr != nil && !os.IsNotExist(readErr) {
		return marshalToolError(call, "read_failed", readErr.Error()), nil
	}
	before := string(beforeBytes)
	// Strip BOM from before for consistent diff display
	before = strings.TrimLeft(before, "\uFEFF")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return marshalToolError(call, "write_failed", err.Error()), nil
	}
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		return marshalToolError(call, "write_failed", err.Error()), nil
	}
	metadata := fileDiffMetadata([]fileChangePreview{{path: in.FilePath, before: before, after: in.Content}})
	return marshalToolResultWithMetadata(call, map[string]any{"file_path": in.FilePath, "bytes": len(in.Content)}, metadata)
}

func (b *Toolset) previewWriteFile(_ context.Context, call core.ToolCall) (map[string]any, error) {
	var in struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := decodeInput(call.Input, &in); err != nil {
		return nil, err
	}
	if in.FilePath == "" {
		return nil, os.ErrInvalid
	}
	abs, err := b.safePath(in.FilePath)
	if err != nil {
		return nil, err
	}
	beforeBytes, err := os.ReadFile(abs)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	before := string(beforeBytes)
	// Strip BOM from before for consistent diff display
	before = strings.TrimLeft(before, "\uFEFF")
	return fileDiffMetadata([]fileChangePreview{{path: in.FilePath, before: before, after: in.Content}}), nil
}
