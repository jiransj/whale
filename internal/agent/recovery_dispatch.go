package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/usewhale/whale/internal/core"
)

func (a *Agent) dispatchWithRecovery(ctx context.Context, sessionID string, call core.ToolCall, events chan<- AgentEvent) (core.ToolResult, bool) {
	attempt := 0
	for {
		attempt++
		res, err := a.tools.Dispatch(ctx, call)
		if ctx.Err() != nil {
			return res, true
		}
		if err != nil {
			res = core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
		}
		class := classifyToolFailure(res, err)
		if class == "" {
			return res, true
		}
		rule, exists := a.recovery.Rules[class]
		if !a.recovery.Enabled || !exists {
			events <- AgentEvent{
				Type: AgentEventTypeToolRecoveryExhausted,
				Recovery: &ToolRecoveryInfo{
					ToolCallID:   call.ID,
					ToolName:     call.Name,
					FailureClass: string(class),
					Action:       string(RecoveryActionRequestReplan),
					Attempt:      attempt,
					MaxAttempts:  0,
					Reason:       "no recovery rule",
				},
			}
			return res, true
		}
		if rule.Action == RecoveryActionFallbackReadOnly {
			fallbackRes, ok := a.executeFallbackReadonly(ctx, call, res)
			if ok {
				events <- AgentEvent{
					Type: AgentEventTypeToolRecoveryExhausted,
					Recovery: &ToolRecoveryInfo{
						ToolCallID:   call.ID,
						ToolName:     call.Name,
						FailureClass: string(class),
						Action:       string(rule.Action),
						Attempt:      attempt,
						MaxAttempts:  rule.MaxAttempts,
						Reason:       res.Content,
						Executed:     true,
					},
				}
				return fallbackRes, true
			}
		}
		if rule.Action == RecoveryActionRequestReplan {
			replanRes := buildRequestReplanResult(call, class, attempt, res.Content)
			events <- AgentEvent{
				Type: AgentEventTypeReplanRequiredSet,
				Recovery: &ToolRecoveryInfo{
					ToolCallID:     call.ID,
					ToolName:       call.Name,
					FailureClass:   string(class),
					Action:         string(rule.Action),
					Attempt:        attempt,
					MaxAttempts:    rule.MaxAttempts,
					Reason:         res.Content,
					ReplanInjected: true,
				},
			}
			events <- AgentEvent{
				Type: AgentEventTypeToolRecoveryExhausted,
				Recovery: &ToolRecoveryInfo{
					ToolCallID:     call.ID,
					ToolName:       call.Name,
					FailureClass:   string(class),
					Action:         string(rule.Action),
					Attempt:        attempt,
					MaxAttempts:    rule.MaxAttempts,
					Reason:         res.Content,
					Executed:       true,
					ReplanInjected: true,
				},
			}
			return replanRes, true
		}
		if attempt > rule.MaxAttempts || rule.Action == RecoveryActionHardBlock {
			events <- AgentEvent{
				Type: AgentEventTypeToolRecoveryExhausted,
				Recovery: &ToolRecoveryInfo{
					ToolCallID:   call.ID,
					ToolName:     call.Name,
					FailureClass: string(class),
					Action:       string(rule.Action),
					Attempt:      attempt,
					MaxAttempts:  rule.MaxAttempts,
					Reason:       res.Content,
				},
			}
			return res, true
		}
		events <- AgentEvent{
			Type: AgentEventTypeToolRecoveryScheduled,
			Recovery: &ToolRecoveryInfo{
				ToolCallID:   call.ID,
				ToolName:     call.Name,
				FailureClass: string(class),
				Action:       string(rule.Action),
				Attempt:      attempt,
				MaxAttempts:  rule.MaxAttempts,
				Reason:       res.Content,
			},
		}
		if rule.Action == RecoveryActionRetryWithBackoff && rule.BackoffMS > 0 {
			timer := time.NewTimer(time.Duration(rule.BackoffMS) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: ctx.Err().Error(), IsError: true}, true
			case <-timer.C:
			}
		}
		events <- AgentEvent{
			Type: AgentEventTypeToolRecoveryAttempt,
			Recovery: &ToolRecoveryInfo{
				ToolCallID:   call.ID,
				ToolName:     call.Name,
				FailureClass: string(class),
				Action:       string(rule.Action),
				Attempt:      attempt + 1,
				MaxAttempts:  rule.MaxAttempts,
			},
		}
	}
}

func (a *Agent) executeFallbackReadonly(ctx context.Context, call core.ToolCall, cause core.ToolResult) (core.ToolResult, bool) {
	fallbackCall := core.ToolCall{ID: call.ID + "-fallback", Name: "list_dir", Input: `{"path":"."}`}
	switch call.Name {
	case "write", "edit":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal([]byte(call.Input), &in) == nil && in.FilePath != "" {
			b, err := json.Marshal(map[string]any{"file_path": in.FilePath})
			if err != nil {
				return core.ToolResult{}, false
			}
			fallbackCall = core.ToolCall{ID: call.ID + "-fallback", Name: "read_file", Input: string(b)}
		}
	case "apply_patch":
		fallbackCall = core.ToolCall{ID: call.ID + "-fallback", Name: "list_dir", Input: `{"path":"."}`}
	case "exec_shell":
		fallbackCall = core.ToolCall{ID: call.ID + "-fallback", Name: "list_dir", Input: `{"path":"."}`}
	default:
		return core.ToolResult{}, false
	}
	res, err := a.tools.Dispatch(ctx, fallbackCall)
	if err != nil {
		return core.ToolResult{}, false
	}
	wrapped, err := json.Marshal(map[string]any{
		"success": true,
		"data": map[string]any{
			"status":  "recovered_with_fallback",
			"summary": "primary tool failed, fallback readonly tool executed",
			"failure": map[string]any{
				"tool":  call.Name,
				"code":  classifyToolFailure(cause, nil),
				"error": cause.Content,
			},
			"fallback": map[string]any{
				"tool":   fallbackCall.Name,
				"result": res.Content,
			},
		},
	})
	if err != nil {
		return core.ToolResult{}, false
	}
	return core.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    string(wrapped),
		IsError:    false,
	}, true
}

func buildRequestReplanResult(call core.ToolCall, class FailureClass, attempt int, reason string) core.ToolResult {
	b, err := json.Marshal(map[string]any{
		"success": false,
		"error":   "recovery exhausted, replan required",
		"code":    "request_replan",
		"data": map[string]any{
			"failure_class":       class,
			"tool_name":           call.Name,
			"tool_call_id":        call.ID,
			"attempts":            attempt,
			"last_error":          reason,
			"suggested_next_step": "Explain the failure and ask the user for direction before retrying.",
		},
	})
	if err != nil {
		return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: `{"success":false,"error":"recovery exhausted, replan required","code":"request_replan"}`, IsError: true}
	}
	return core.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    string(b),
		IsError:    true,
	}
}
