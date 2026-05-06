package app

import (
	"fmt"
	"strings"

	"github.com/usewhale/whale/internal/agent"
)

func renderHookReport(report agent.HookReport) []string {
	out := make([]string, 0)
	for _, oc := range report.Outcomes {
		if oc.Decision == agent.HookDecisionPass {
			continue
		}
		msg := strings.TrimSpace(oc.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(oc.Stdout)
		}
		out = append(out, fmt.Sprintf("[hook] event:%s decision:%s cmd:%s code:%d duration_ms:%d truncated:%v msg:%s", report.Event, oc.Decision, oc.Hook.Command, oc.ExitCode, oc.DurationMS, oc.Truncated, msg))
	}
	return out
}
