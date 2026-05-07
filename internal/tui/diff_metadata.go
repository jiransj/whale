package tui

import (
	"fmt"
	"strings"
)

func renderFileDiffMetadataMarkdown(metadata map[string]any, maxLines int) string {
	plain := renderFileDiffMetadataPlain(metadata, maxLines)
	if strings.TrimSpace(plain) == "" {
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(plain), "diff preview unavailable:") {
		return plain
	}
	return "```diff\n" + plain + "\n```"
}

func renderFileDiffMetadataPlain(metadata map[string]any, maxLines int) string {
	if len(metadata) == 0 || strings.TrimSpace(asString(metadata["kind"])) != "file_diff" {
		return ""
	}
	if msg := strings.TrimSpace(asString(metadata["preview_error"])); msg != "" {
		return "diff preview unavailable: " + msg
	}
	files := fileDiffMetadataFiles(metadata["files"])
	if len(files) == 0 {
		return ""
	}
	lines := make([]string, 0, len(files)*8)
	for _, file := range files {
		if strings.TrimSpace(file.diff) == "" {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, strings.Split(strings.TrimRight(file.diff, "\n"), "\n")...)
		if file.truncated {
			lines = append(lines, "... diff truncated ...")
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if maxLines > 0 && len(lines) > maxLines {
		hidden := len(lines) - maxLines
		lines = append(lines[:maxLines], fmt.Sprintf("... diff truncated (%d lines hidden) ...", hidden))
	}
	return strings.Join(lines, "\n")
}

type fileDiffView struct {
	path      string
	diff      string
	truncated bool
}

func fileDiffMetadataFiles(value any) []fileDiffView {
	switch files := value.(type) {
	case []map[string]any:
		out := make([]fileDiffView, 0, len(files))
		for _, file := range files {
			out = append(out, fileDiffView{
				path:      asString(file["path"]),
				diff:      asString(file["unified_diff"]),
				truncated: asBool(file["truncated"]),
			})
		}
		return out
	case []any:
		out := make([]fileDiffView, 0, len(files))
		for _, item := range files {
			file, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, fileDiffView{
				path:      asString(file["path"]),
				diff:      asString(file["unified_diff"]),
				truncated: asBool(file["truncated"]),
			})
		}
		return out
	default:
		return nil
	}
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}
