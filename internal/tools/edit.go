package tools

import (
	"bytes"
	"context"
	"os"
	"strings"

	"github.com/usewhale/whale/internal/core"
)

func (b *Toolset) editFile(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	var in struct {
		FilePath string `json:"file_path"`
		Search   string `json:"search"`
		Replace  string `json:"replace"`
		All      bool   `json:"all"`
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
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return marshalToolError(call, "not_found", err.Error()), nil
		}
		return marshalToolError(call, "read_failed", err.Error()), nil
	}
	// Strip UTF-8 BOM (0xEF 0xBB 0xBF) that some Windows editors add.
	// Without this, read_file output includes a visible \uFEFF character at
	// the start of the first line, but LLMs often omit this invisible char
	// when constructing the search string, causing "search text not found".
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	// Normalize CRLF→LF so that search text from readFile output (which is LF-only)
	// matches file content on Windows where files typically use CRLF.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	// Handle standalone \r (Classic Mac style) for consistency with read_file.
	data = bytes.ReplaceAll(data, []byte("\r"), []byte{})
	before := string(data)
	if in.Search == "" {
		return marshalToolError(call, "invalid_args", "search is required"), nil
	}
	if !strings.Contains(before, in.Search) {
		return marshalToolError(call, "search_not_found", "search text not found"), nil
	}
	after := ""
	replacements := 1
	if in.All {
		replacements = strings.Count(before, in.Search)
		after = strings.ReplaceAll(before, in.Search, in.Replace)
	} else {
		after = strings.Replace(before, in.Search, in.Replace, 1)
	}
	if err := os.WriteFile(abs, []byte(after), 0o644); err != nil {
		return marshalToolError(call, "write_failed", err.Error()), nil
	}
	// Note: after is LF-only. On Windows, Go's WriteFile writes bytes as-is.
	// This ensures consistent line endings across edit operations,
	// matching the LF-only output that read_file displays.
	metadata := fileDiffMetadata([]fileChangePreview{{path: in.FilePath, before: before, after: after}})
	return marshalToolResultWithMetadata(call, map[string]any{
		"file_path":    in.FilePath,
		"replacements": replacements,
	}, metadata)
}

func (b *Toolset) previewEditFile(_ context.Context, call core.ToolCall) (map[string]any, error) {
	var in struct {
		FilePath string `json:"file_path"`
		Search   string `json:"search"`
		Replace  string `json:"replace"`
		All      bool   `json:"all"`
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
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	// Strip UTF-8 BOM for preview matching consistency with read_file.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	// Normalize CRLF→LF for preview matching consistency
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	// Handle standalone \r for consistency with read_file.
	data = bytes.ReplaceAll(data, []byte("\r"), []byte{})
	before := string(data)
	if in.Search == "" {
		return nil, os.ErrInvalid
	}
	if !strings.Contains(before, in.Search) {
		return nil, os.ErrNotExist
	}
	after := strings.Replace(before, in.Search, in.Replace, 1)
	if in.All {
		after = strings.ReplaceAll(before, in.Search, in.Replace)
	}
	return fileDiffMetadata([]fileChangePreview{{path: in.FilePath, before: before, after: after}}), nil
}
