package tasks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usewhale/whale/internal/core"
)

func emitSubagentProgress(progress func(core.ToolProgress), role, model string, count int, status, summary string, metadata map[string]any) {
	if progress == nil {
		return
	}
	progress(core.ToolProgress{
		ToolName: "spawn_subagent",
		Role:     role,
		Model:    model,
		Count:    count,
		Status:   status,
		Summary:  strings.TrimSpace(summary),
		Metadata: metadata,
	})
}

type childToolAction struct {
	ToolName string
	Target   string
	Running  string
	DoneVerb string
}

func summarizeChildToolCall(call core.ToolCall) childToolAction {
	var args map[string]any
	_ = json.Unmarshal([]byte(call.Input), &args)
	switch call.Name {
	case "read_file":
		target := compactProgressTarget(firstNonEmptyString(asString(args["file_path"]), asString(args["path"]), "file"))
		return childToolAction{ToolName: call.Name, Target: target, Running: "Reading " + target, DoneVerb: "Read"}
	case "list_dir":
		target := compactProgressTarget(firstNonEmptyString(asString(args["path"]), "."))
		return childToolAction{ToolName: call.Name, Target: target, Running: "Listing " + target, DoneVerb: "Listed"}
	case "grep", "search_content":
		target := summarizeSearchTarget(args)
		return childToolAction{ToolName: call.Name, Target: target, Running: "Searching " + target, DoneVerb: "Searched"}
	case "search_files":
		pattern := quoteProgressTerm(firstNonEmptyString(asString(args["pattern"]), asString(args["query"]), "files"))
		return childToolAction{ToolName: call.Name, Target: pattern, Running: "Searching files " + pattern, DoneVerb: "Searched files"}
	case "web_search":
		target := quoteProgressTerm(firstNonEmptyString(asString(args["query"]), "query"))
		return childToolAction{ToolName: call.Name, Target: target, Running: "Searching web " + target, DoneVerb: "Searched web"}
	case "fetch", "web_fetch":
		target := compactURLForProgress(firstNonEmptyString(asString(args["url"]), "url"))
		return childToolAction{ToolName: call.Name, Target: target, Running: "Fetching " + target, DoneVerb: "Fetched"}
	default:
		if call.Name != "" {
			return childToolAction{ToolName: call.Name, Target: call.Name, Running: "Using " + call.Name, DoneVerb: "Used"}
		}
		return childToolAction{ToolName: call.Name, Target: "tool", Running: "Using tool", DoneVerb: "Used"}
	}
}

func summarizeSearchTarget(args map[string]any) string {
	pattern := quoteProgressTerm(firstNonEmptyString(asString(args["pattern"]), asString(args["query"]), "content"))
	path := compactProgressTarget(firstNonEmptyString(asString(args["path"]), asString(args["directory"]), ""))
	include := compactProgressTarget(firstNonEmptyString(asString(args["include"]), ""))
	if path != "" && include != "" {
		return fmt.Sprintf("%s in %s (%s)", pattern, path, include)
	}
	if path != "" {
		return fmt.Sprintf("%s in %s", pattern, path)
	}
	if include != "" {
		return fmt.Sprintf("%s (%s)", pattern, include)
	}
	return pattern
}

func summarizeChildToolResult(res core.ToolResult, action childToolAction) string {
	if res.IsError {
		if action.Target != "" {
			return action.DoneVerb + " " + action.Target + " failed"
		}
		return res.Name + " failed"
	}
	if action.Target == "" {
		return res.Name + " completed"
	}
	summary := action.DoneVerb + " " + action.Target
	if suffix := childResultMetricSuffix(res); suffix != "" {
		summary += " · " + suffix
	}
	return summary
}

func childResultMetricSuffix(res core.ToolResult) string {
	env, ok := core.ParseToolEnvelope(res.Content)
	if !ok || !env.OK || !env.Success {
		return ""
	}
	metrics := asMap(env.Data["metrics"])
	payload := asMap(env.Data["payload"])
	switch res.Name {
	case "read_file":
		total := asInt(metrics["total_lines"])
		returned := asInt(metrics["returned_lines"])
		if total > 0 && returned > 0 {
			return fmt.Sprintf("%d/%d lines", returned, total)
		}
	case "list_dir":
		items := asAnySlice(payload["items"])
		if len(items) == 0 {
			items = asAnySlice(env.Data["items"])
		}
		if len(items) > 0 {
			return fmt.Sprintf("%d items", len(items))
		}
	case "grep", "search_content":
		total := asInt(metrics["total_matches"])
		files := asInt(metrics["files_matched"])
		if files > 0 {
			return fmt.Sprintf("%d matches in %d files", total, files)
		}
		if total >= 0 {
			return fmt.Sprintf("%d matches", total)
		}
	case "search_files":
		total := asInt(metrics["total_matches"])
		if total > 0 {
			return fmt.Sprintf("%d matches", total)
		}
		items := asAnySlice(payload["items"])
		if len(items) > 0 {
			return fmt.Sprintf("%d matches", len(items))
		}
	case "web_search":
		count := asInt(env.Data["count"])
		if count > 0 {
			return fmt.Sprintf("%d results", count)
		}
	case "fetch", "web_fetch":
		status := asInt(env.Data["status_code"])
		if status > 0 {
			return fmt.Sprintf("HTTP %d", status)
		}
	}
	return ""
}
