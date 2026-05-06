package agent

import (
	"context"

	"github.com/usewhale/whale/internal/core"
)

const interruptedTurnMarkerText = "<turn_aborted>\nThe user interrupted the previous turn on purpose. Any running tools or commands may have partially executed; verify current state before retrying.\n</turn_aborted>"

func (a *Agent) persistInterruptedTurnMarker(sessionID string) {
	_, _ = a.store.Create(context.Background(), core.Message{
		SessionID:    sessionID,
		Role:         core.RoleUser,
		Text:         interruptedTurnMarkerText,
		Hidden:       true,
		FinishReason: core.FinishReasonCanceled,
	})
}
