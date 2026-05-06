package app

import (
	"fmt"
	"strings"

	"github.com/usewhale/whale/internal/agent"
)

func formatHookOutcomeLine(event agent.HookEvent, oc agent.HookOutcome) string {
	msg := strings.TrimSpace(oc.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(oc.Stdout)
	}
	return fmt.Sprintf(
		"[hook] event:%s decision:%s cmd:%s code:%d duration_ms:%d truncated:%v msg:%s",
		event,
		oc.Decision,
		oc.Hook.Command,
		oc.ExitCode,
		oc.DurationMS,
		oc.Truncated,
		msg,
	)
}

func formatHookEventLine(tag string, h *agent.HookEventInfo) string {
	if h == nil {
		return "[hook] <nil>"
	}
	decision := strings.TrimSpace(tag)
	if h.Decision != "" {
		decision = string(h.Decision)
	}
	return fmt.Sprintf(
		"[hook] event:%s decision:%s cmd:%s code:%d duration_ms:%d truncated:%v msg:%s",
		h.Event,
		decision,
		h.Name,
		h.ExitCode,
		h.DurationMS,
		h.Truncated,
		strings.TrimSpace(h.Message),
	)
}
