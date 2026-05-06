package agent

import "strings"

func (a *Agent) emitHookReport(events chan<- AgentEvent, report HookReport) {
	for _, oc := range report.Outcomes {
		events <- AgentEvent{
			Type: AgentEventTypeHookStarted,
			Hook: &HookEventInfo{
				Name:  oc.Hook.Command,
				Event: report.Event,
			},
		}
		msg := strings.TrimSpace(oc.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(oc.Stdout)
		}
		switch oc.Decision {
		case HookDecisionBlock:
			events <- AgentEvent{
				Type: AgentEventTypeHookBlocked,
				Hook: &HookEventInfo{
					Name:       oc.Hook.Command,
					Event:      report.Event,
					Decision:   oc.Decision,
					ExitCode:   oc.ExitCode,
					Message:    msg,
					DurationMS: oc.DurationMS,
					Truncated:  oc.Truncated,
				},
			}
		case HookDecisionError, HookDecisionTimeout:
			events <- AgentEvent{
				Type: AgentEventTypeHookFailed,
				Hook: &HookEventInfo{
					Name:       oc.Hook.Command,
					Event:      report.Event,
					Decision:   oc.Decision,
					ExitCode:   oc.ExitCode,
					Message:    msg,
					DurationMS: oc.DurationMS,
					Truncated:  oc.Truncated,
				},
			}
		case HookDecisionWarn:
			events <- AgentEvent{
				Type: AgentEventTypeHookWarned,
				Hook: &HookEventInfo{
					Name:       oc.Hook.Command,
					Event:      report.Event,
					Decision:   oc.Decision,
					ExitCode:   oc.ExitCode,
					Message:    msg,
					DurationMS: oc.DurationMS,
					Truncated:  oc.Truncated,
				},
			}
		}
		events <- AgentEvent{
			Type: AgentEventTypeHookCompleted,
			Hook: &HookEventInfo{
				Name:       oc.Hook.Command,
				Event:      report.Event,
				Decision:   oc.Decision,
				ExitCode:   oc.ExitCode,
				Message:    msg,
				DurationMS: oc.DurationMS,
				Truncated:  oc.Truncated,
			},
		}
	}
}
