package agent

import (
	"strings"
	"time"

	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/telemetry"
)

const (
	toolInputEventRepaired = "tool_input_repaired"
	toolInputEventInvalid  = "tool_input_invalid"
)

func (a *Agent) recordToolInputRepair(sessionID, model, assistantMessageID string, call core.ToolCall, repairKind string) {
	a.recordToolInputEvent(telemetry.ToolInputEvent{
		Session:            sessionID,
		Model:              model,
		AssistantMessageID: assistantMessageID,
		ToolCallID:         call.ID,
		Tool:               call.Name,
		Event:              toolInputEventRepaired,
		RepairKind:         repairKind,
	})
}

func (a *Agent) recordToolInputInvalid(sessionID, model, assistantMessageID string, call core.ToolCall, errorCode string) {
	a.recordToolInputEvent(telemetry.ToolInputEvent{
		Session:            sessionID,
		Model:              model,
		AssistantMessageID: assistantMessageID,
		ToolCallID:         call.ID,
		Tool:               call.Name,
		Event:              toolInputEventInvalid,
		ErrorCode:          errorCode,
	})
}

func (a *Agent) recordToolInputEvent(rec telemetry.ToolInputEvent) {
	if a == nil || strings.TrimSpace(a.sessionsDir) == "" {
		return
	}
	_ = telemetry.AppendToolInputEvent(a.sessionsDir, rec, time.Now())
}

func toolInputInvalidCode(res core.ToolResult) string {
	if !res.IsError {
		return ""
	}
	env, ok := core.ParseToolEnvelope(res.Content)
	if !ok {
		return ""
	}
	switch strings.TrimSpace(env.Code) {
	case "invalid_input", "invalid_args":
		return strings.TrimSpace(env.Code)
	default:
		return ""
	}
}
