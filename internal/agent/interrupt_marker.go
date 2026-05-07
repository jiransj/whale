package agent

import (
	"context"

	"github.com/usewhale/whale/internal/core"
)

const interruptedTurnMarkerText = "<turn_aborted>\nThe user interrupted the previous turn on purpose. Any running tools or commands may have partially executed; verify current state before retrying.\n</turn_aborted>"

func approvalDeniedMarkerText(toolName string) string {
	if toolName == "" {
		toolName = "unknown"
	}
	return "<approval_denied>\nThe user denied a requested tool/action (tool: " + toolName + "). Treat the related task path as canceled. Do not retry or continue the denied action or its parent task unless the user explicitly asks.\n</approval_denied>"
}

func (a *Agent) persistInterruptedTurnMarker(sessionID string) {
	_, _ = a.store.Create(context.Background(), core.Message{
		SessionID:    sessionID,
		Role:         core.RoleUser,
		Text:         interruptedTurnMarkerText,
		Hidden:       true,
		FinishReason: core.FinishReasonCanceled,
	})
}

func (a *Agent) persistApprovalDeniedMarker(sessionID, toolName string) {
	_, _ = a.store.Create(context.Background(), core.Message{
		SessionID:    sessionID,
		Role:         core.RoleUser,
		Text:         approvalDeniedMarkerText(toolName),
		Hidden:       true,
		FinishReason: core.FinishReasonCanceled,
	})
}
