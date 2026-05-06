package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/usewhale/whale/internal/core"
)

type patchOpType string

const (
	patchOpUpdate patchOpType = "update"
	patchOpAdd    patchOpType = "add"
	patchOpDelete patchOpType = "delete"
)

type patchHunk struct {
	oldLines []string
	newLines []string
}

type patchOp struct {
	kind  patchOpType
	path  string
	hunks []patchHunk
	added []string
}

func (b *Toolset) applyPatch(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	var in struct {
		Patch string `json:"patch"`
	}
	if err := decodeInput(call.Input, &in); err != nil {
		return marshalToolError(call, "invalid_args", err.Error()), nil
	}
	if strings.TrimSpace(in.Patch) == "" {
		return marshalToolError(call, "invalid_args", "patch is required"), nil
	}

	ops, err := parseBeginPatch(in.Patch)
	if err != nil {
		return marshalToolError(call, "patch_parse_failed", err.Error()), nil
	}
	filesChanged := make([]string, 0, len(ops))
	additions := 0
	deletions := 0

	for _, op := range ops {
		abs, err := b.safePath(op.path)
		if err != nil {
			return marshalToolError(call, "permission_denied", err.Error()), nil
		}
		switch op.kind {
		case patchOpAdd:
			if _, err := os.Stat(abs); err == nil {
				return marshalToolError(call, "patch_apply_failed", "add file already exists: "+op.path), nil
			}
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return marshalToolError(call, "patch_apply_failed", err.Error()), nil
			}
			content := strings.Join(op.added, "\n")
			if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
				return marshalToolError(call, "patch_apply_failed", err.Error()), nil
			}
			additions += len(op.added)
			filesChanged = append(filesChanged, op.path)
		case patchOpDelete:
			if err := os.Remove(abs); err != nil {
				if os.IsNotExist(err) {
					return marshalToolError(call, "patch_apply_failed", "delete target missing: "+op.path), nil
				}
				return marshalToolError(call, "patch_apply_failed", err.Error()), nil
			}
			filesChanged = append(filesChanged, op.path)
		case patchOpUpdate:
			raw, err := os.ReadFile(abs)
			if err != nil {
				if os.IsNotExist(err) {
					return marshalToolError(call, "patch_apply_failed", "update target missing: "+op.path), nil
				}
				return marshalToolError(call, "patch_apply_failed", err.Error()), nil
			}
			lines, hadTrailingNewline := splitLinesKeepFlag(string(raw))
			next := make([]string, len(lines))
			copy(next, lines)
			for _, h := range op.hunks {
				idx := findSubslice(next, h.oldLines)
				if idx < 0 {
					return marshalToolError(call, "patch_apply_failed", fmt.Sprintf("hunk context not found in %s", op.path)), nil
				}
				before := append([]string{}, next[:idx]...)
				after := append([]string{}, next[idx+len(h.oldLines):]...)
				next = append(before, append(h.newLines, after...)...)
				if len(h.newLines) > len(h.oldLines) {
					additions += len(h.newLines) - len(h.oldLines)
				} else {
					deletions += len(h.oldLines) - len(h.newLines)
				}
			}
			out := strings.Join(next, "\n")
			if hadTrailingNewline {
				out += "\n"
			}
			if err := os.WriteFile(abs, []byte(out), 0o644); err != nil {
				return marshalToolError(call, "patch_apply_failed", err.Error()), nil
			}
			filesChanged = append(filesChanged, op.path)
		}
	}

	return marshalToolResult(call, map[string]any{
		"files_changed": filesChanged,
		"additions":     additions,
		"deletions":     deletions,
	})
}

func parseBeginPatch(patch string) ([]patchOp, error) {
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "*** Begin Patch" {
		return nil, fmt.Errorf("missing *** Begin Patch")
	}
	i++
	ops := make([]patchOp, 0)
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "*** End Patch" {
			return ops, nil
		}
		switch {
		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			if path == "" {
				return nil, fmt.Errorf("empty update path")
			}
			i++
			hunks := make([]patchHunk, 0)
			for i < len(lines) {
				if strings.HasPrefix(lines[i], "*** ") || strings.TrimSpace(lines[i]) == "*** End Patch" {
					break
				}
				if strings.HasPrefix(lines[i], "@@") {
					i++
					oldLines := make([]string, 0)
					newLines := make([]string, 0)
					for i < len(lines) {
						l := lines[i]
						if strings.HasPrefix(l, "@@") || strings.HasPrefix(l, "*** ") || strings.TrimSpace(l) == "*** End Patch" {
							break
						}
						if strings.HasPrefix(l, "-") {
							oldLines = append(oldLines, strings.TrimPrefix(l, "-"))
						} else if strings.HasPrefix(l, "+") {
							newLines = append(newLines, strings.TrimPrefix(l, "+"))
						} else if strings.HasPrefix(l, " ") {
							v := strings.TrimPrefix(l, " ")
							oldLines = append(oldLines, v)
							newLines = append(newLines, v)
						} else if l == `\ No newline at end of file` {
							// ignore marker
						} else {
							return nil, fmt.Errorf("invalid hunk line: %s", l)
						}
						i++
					}
					if len(oldLines) == 0 && len(newLines) == 0 {
						return nil, fmt.Errorf("empty hunk for %s", path)
					}
					hunks = append(hunks, patchHunk{oldLines: oldLines, newLines: newLines})
					continue
				}
				i++
			}
			if len(hunks) == 0 {
				return nil, fmt.Errorf("update file without hunks: %s", path)
			}
			ops = append(ops, patchOp{kind: patchOpUpdate, path: path, hunks: hunks})
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			if path == "" {
				return nil, fmt.Errorf("empty add path")
			}
			i++
			added := make([]string, 0)
			for i < len(lines) {
				l := lines[i]
				if strings.HasPrefix(l, "*** ") || strings.TrimSpace(l) == "*** End Patch" {
					break
				}
				if strings.HasPrefix(l, "+") {
					added = append(added, strings.TrimPrefix(l, "+"))
				} else {
					return nil, fmt.Errorf("invalid add line: %s", l)
				}
				i++
			}
			ops = append(ops, patchOp{kind: patchOpAdd, path: path, added: added})
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			if path == "" {
				return nil, fmt.Errorf("empty delete path")
			}
			ops = append(ops, patchOp{kind: patchOpDelete, path: path})
			i++
		default:
			if strings.TrimSpace(line) == "" {
				i++
				continue
			}
			return nil, fmt.Errorf("unknown patch line: %s", line)
		}
	}
	return nil, fmt.Errorf("missing *** End Patch")
}

func splitLinesKeepFlag(s string) ([]string, bool) {
	if s == "" {
		return []string{}, false
	}
	hadTrailing := strings.HasSuffix(s, "\n")
	trimmed := strings.TrimSuffix(s, "\n")
	return strings.Split(trimmed, "\n"), hadTrailing
}

func findSubslice(haystack, needle []string) int {
	if len(needle) == 0 {
		return 0
	}
	if len(needle) > len(haystack) {
		return -1
	}
outer:
	for i := 0; i <= len(haystack)-len(needle); i++ {
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
