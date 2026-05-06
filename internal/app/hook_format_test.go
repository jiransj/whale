package app

import (
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/agent"
)

func TestFormatHookOutcomeLineIncludesCoreFields(t *testing.T) {
	line := formatHookOutcomeLine(agent.HookEventPreToolUse, agent.HookOutcome{
		Hook:       agent.ResolvedHook{HookConfig: agent.HookConfig{Command: "echo hi"}},
		Decision:   agent.HookDecisionWarn,
		ExitCode:   9,
		Stderr:     "problem",
		DurationMS: 42,
		Truncated:  true,
	})
	for _, p := range []string{
		"event:PreToolUse",
		"decision:warn",
		"cmd:echo hi",
		"code:9",
		"duration_ms:42",
		"truncated:true",
		"msg:problem",
	} {
		if !strings.Contains(line, p) {
			t.Fatalf("missing %q in %q", p, line)
		}
	}
}

func TestFormatHookEventLinePrefersDecisionField(t *testing.T) {
	line := formatHookEventLine("started", &agent.HookEventInfo{
		Event:      agent.HookEventStop,
		Decision:   agent.HookDecisionTimeout,
		Name:       "echo end",
		ExitCode:   -1,
		DurationMS: 100,
		Truncated:  false,
		Message:    "timeout",
	})
	if !strings.Contains(line, "decision:timeout") {
		t.Fatalf("expected decision from struct, got %q", line)
	}
}
