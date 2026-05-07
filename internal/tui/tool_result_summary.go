package tui

import (
	"fmt"
	"strings"
)

const shellOutputPreviewLines = 6
const shellOutputHeadLines = 2
const shellOutputTailLines = 2
const shellOutputLineRunes = 220

func summarizeToolResultForChat(toolName, raw string) (string, string) {
	body := strings.TrimSpace(raw)
	if body == "" {
		return "result", body
	}
	env, ok := parseToolEnvelopeOK(body)
	if !ok {
		return "result_error", "ERROR · malformed tool result"
	}

	successBySignal := env.success
	if !env.hasSuccess {
		switch {
		case env.hasOK && env.ok:
			successBySignal = true
		case env.code == "ok":
			successBySignal = true
		default:
			successBySignal = true
		}
	}
	if env.status != "" && env.status != "ok" && env.status != "running" && env.status != "done" && env.status != "completed" && env.status != "success" {
		successBySignal = false
	}

	switch toolDisplayKind(toolName) {
	case "shell":
		return summarizeShellResult(env, successBySignal)
	case "explore":
		return summarizeExploreResult(toolName, env, successBySignal)
	case "edit":
		return summarizeEditResult(toolName, env, successBySignal)
	default:
		if !successBySignal {
			return summarizeFailedResult(env, "tool failed")
		}
		return "result_ok", "✓"
	}
}

type toolResultEnvelope struct {
	success    bool
	hasSuccess bool
	ok         bool
	hasOK      bool
	code       string
	message    string
	summary    string
	status     string
	data       map[string]any
	metrics    map[string]any
	payload    map[string]any
	metadata   map[string]any
}

func summarizeShellResult(env toolResultEnvelope, successBySignal bool) (string, string) {
	exitCode := asInt(env.metrics["exit_code"])
	hasExitCode := hasInt(env.metrics["exit_code"])
	duration := formatDurationMS(asInt64(env.metrics["duration_ms"]))
	if env.status == "running" {
		if duration != "" {
			return "result_running", "running · " + duration
		}
		return "result_running", "running"
	}

	if !successBySignal {
		return summarizeFailedResult(env, "command failed")
	}

	_ = exitCode
	_ = hasExitCode
	parts := []string{"✓"}
	if duration != "" {
		parts = append(parts, duration)
	}
	output := summarizeShellOutput(firstNonEmpty(asString(env.payload["stdout"]), asString(env.payload["stderr"])))
	if output != "" {
		return "result_ok", strings.Join(parts, " · ") + "\n" + output
	}
	return "result_ok", strings.Join(parts, " · ")
}

func summarizeShellOutput(text string) string {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		lines = append(lines, truncateShellOutputLine(strings.TrimRight(line, "\r")))
	}
	if len(lines) <= shellOutputPreviewLines {
		return strings.Join(lines, "\n")
	}
	head := minInt(shellOutputHeadLines, len(lines))
	tail := minInt(shellOutputTailLines, len(lines)-head)
	omitted := len(lines) - head - tail
	out := make([]string, 0, head+1+tail)
	out = append(out, lines[:head]...)
	out = append(out, fmt.Sprintf("... %d lines omitted; use /tool for full output", omitted))
	out = append(out, lines[len(lines)-tail:]...)
	return strings.Join(out, "\n")
}

func truncateShellOutputLine(line string) string {
	runes := []rune(line)
	if len(runes) <= shellOutputLineRunes {
		return line
	}
	return string(runes[:shellOutputLineRunes]) + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func summarizeFailedResult(env toolResultEnvelope, fallback string) (string, string) {
	exitCode := asInt(env.metrics["exit_code"])
	hasExitCode := hasInt(env.metrics["exit_code"])
	duration := formatDurationMS(asInt64(env.metrics["duration_ms"]))
	detail := firstLine(firstNonEmpty(
		env.summary,
		asString(env.payload["stderr"]),
		asString(env.payload["stdout"]),
		env.message,
		asString(env.data["summary"]),
		fallback,
	))

	switch env.code {
	case "approval_denied", "policy_denied", "permission_denied":
		return "result_denied", "DENIED · " + detail
	case "timeout":
		if duration != "" {
			return "result_timeout", "TIMEOUT · " + duration
		}
		return "result_timeout", "TIMEOUT"
	case "cancelled", "canceled":
		return "result_canceled", "CANCELED"
	}

	lower := strings.ToLower(detail + " " + env.code)
	if strings.Contains(lower, "denied") || strings.Contains(lower, "approval") || strings.Contains(lower, "policy") {
		return "result_denied", "DENIED · " + detail
	}
	if strings.Contains(lower, "timeout") {
		if duration != "" {
			return "result_timeout", "TIMEOUT · " + duration
		}
		return "result_timeout", "TIMEOUT"
	}
	if strings.Contains(lower, "cancel") {
		return "result_canceled", "CANCELED"
	}

	prefix := "✗"
	if hasExitCode && exitCode > 0 {
		prefix = fmt.Sprintf("✗ (exit %d)", exitCode)
	}
	if duration != "" {
		return "result_failed", fmt.Sprintf("%s · %s · %s", prefix, duration, detail)
	}
	return "result_failed", fmt.Sprintf("%s · %s", prefix, detail)
}

func summarizeExploreResult(toolName string, env toolResultEnvelope, successBySignal bool) (string, string) {
	if !successBySignal {
		return summarizeFailedResult(env, "exploration failed")
	}
	switch toolName {
	case "read_file":
		total := asInt(env.metrics["total_lines"])
		returned := asInt(env.metrics["returned_lines"])
		if total > 0 {
			return "result_ok", fmt.Sprintf("✓ · %d/%d lines", returned, total)
		}
	case "list_dir":
		items := stringSlice(firstNonEmptyAny(env.payload["items"], env.data["items"]))
		return "result_ok", fmt.Sprintf("✓ · %d items", len(items))
	case "search_files":
		total := asInt(env.metrics["total_matches"])
		if total > 0 {
			return "result_ok", fmt.Sprintf("✓ · %d matches", total)
		}
		items := stringSlice(firstNonEmptyAny(env.payload["items"], env.data["items"]))
		return "result_ok", fmt.Sprintf("✓ · %d matches", len(items))
	case "grep", "search_content":
		total := asInt(env.metrics["total_matches"])
		files := asInt(env.metrics["files_matched"])
		if files > 0 {
			return "result_ok", fmt.Sprintf("✓ · %d matches in %d files", total, files)
		}
		return "result_ok", fmt.Sprintf("✓ · %d matches", total)
	case "fetch", "web_fetch":
		status := asInt(firstNonEmptyAny(env.payload["status_code"], env.data["status_code"]))
		format := firstNonEmpty(asString(env.payload["format"]), asString(env.data["format"]))
		if status > 0 && format != "" {
			return "result_ok", fmt.Sprintf("✓ · HTTP %d · %s", status, format)
		}
	case "web_search":
		return "result_ok", "✓"
	}
	return "result_ok", "✓"
}

func summarizeEditResult(toolName string, env toolResultEnvelope, successBySignal bool) (string, string) {
	if !successBySignal {
		return summarizeFailedResult(env, "edit failed")
	}
	switch toolName {
	case "write_file", "write":
		if n := asInt(firstNonEmptyAny(env.payload["bytes"], env.data["bytes"])); n > 0 {
			return "result_ok", fmt.Sprintf("✓ · %d bytes", n)
		}
	case "edit_file", "edit":
		if n := asInt(firstNonEmptyAny(env.payload["replacements"], env.data["replacements"])); n > 0 {
			return "result_ok", fmt.Sprintf("✓ · %d replacements", n)
		}
	case "apply_patch":
		additions := asInt(firstNonEmptyAny(env.payload["additions"], env.data["additions"]))
		deletions := asInt(firstNonEmptyAny(env.payload["deletions"], env.data["deletions"]))
		if additions > 0 || deletions > 0 {
			return "result_ok", fmt.Sprintf("✓ · +%d -%d", additions, deletions)
		}
	}
	return "result_ok", "✓"
}
