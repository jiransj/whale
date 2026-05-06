package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/usewhale/whale/internal/core"
)

func (b *Toolset) searchContent(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	var in struct {
		Path        string `json:"path"`
		Pattern     string `json:"pattern"`
		Include     string `json:"include"`
		LiteralText bool   `json:"literal_text"`
	}
	if err := decodeInput(call.Input, &in); err != nil {
		return marshalToolError(call, "invalid_args", err.Error()), nil
	}
	if strings.TrimSpace(in.Pattern) == "" {
		return marshalToolError(call, "invalid_args", "pattern is required"), nil
	}
	abs, err := b.safePath(in.Path)
	if err != nil {
		return marshalToolError(call, "permission_denied", err.Error()), nil
	}
	args := []string{"-n", "--no-heading"}
	args = append(args, "--json")
	if in.LiteralText {
		args = append(args, "-F")
	}
	if strings.TrimSpace(in.Include) != "" {
		args = append(args, "-g", in.Include)
	}
	args = append(args, in.Pattern, abs)
	cmd := exec.Command("rg", args...)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return marshalToolError(call, "exec_failed", err.Error()), nil
	}

	type submatch struct {
		Match string `json:"match"`
		Start int    `json:"start"`
		End   int    `json:"end"`
	}
	type matchRow struct {
		File       string     `json:"file"`
		LineNumber int        `json:"line_number"`
		Line       string     `json:"line"`
		Submatches []submatch `json:"submatches"`
	}

	var matches []matchRow
	byFile := map[string]int{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		var evt map[string]any
		if json.Unmarshal([]byte(line), &evt) != nil {
			continue
		}
		if evt["type"] != "match" {
			continue
		}
		data, _ := evt["data"].(map[string]any)
		pathObj, _ := data["path"].(map[string]any)
		textObj, _ := data["lines"].(map[string]any)
		rawPath, _ := pathObj["text"].(string)
		rawLine, _ := textObj["text"].(string)
		num, _ := data["line_number"].(float64)

		rel := rawPath
		if rp, rerr := filepath.Rel(b.root, rawPath); rerr == nil {
			rel = filepath.ToSlash(rp)
		}
		row := matchRow{
			File:       rel,
			LineNumber: int(num),
			Line:       strings.TrimRight(rawLine, "\n"),
		}
		if sms, ok := data["submatches"].([]any); ok {
			for _, one := range sms {
				obj, _ := one.(map[string]any)
				mobj, _ := obj["match"].(map[string]any)
				mv, _ := mobj["text"].(string)
				sv, _ := obj["start"].(float64)
				ev, _ := obj["end"].(float64)
				row.Submatches = append(row.Submatches, submatch{Match: mv, Start: int(sv), End: int(ev)})
			}
		}
		matches = append(matches, row)
		byFile[row.File]++
	}
	if err := sc.Err(); err != nil {
		return marshalToolError(call, "parse_failed", err.Error()), nil
	}
	summaryParts := make([]string, 0, maxSummarySamples)
	for f, c := range byFile {
		summaryParts = append(summaryParts, f+":"+strconv.Itoa(c))
		if len(summaryParts) >= maxSummarySamples {
			break
		}
	}
	result := map[string]any{
		"status": "ok",
		"metrics": map[string]any{
			"total_matches":  len(matches),
			"files_matched":  len(byFile),
			"pattern_length": len([]rune(in.Pattern)),
			"truncated":      false,
		},
		"payload": map[string]any{
			"matches": matches,
		},
		"summary": strings.Join(summaryParts, " | "),
	}
	return marshalToolResult(call, result)
}
