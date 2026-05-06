package agent

import (
	"context"

	"github.com/usewhale/whale/internal/store"
)

func (a *Agent) ensureApprovalCacheLoaded(ctx context.Context, sessionID string) {
	if a.approvalCache.IsLoaded(sessionID) {
		return
	}
	as, ok := a.store.(store.ApprovalStore)
	if !ok {
		a.approvalCache.SetLoaded(sessionID)
		return
	}
	keys, err := as.GetApprovals(ctx, sessionID)
	if err == nil {
		a.approvalCache.Merge(sessionID, keys)
	}
	a.approvalCache.SetLoaded(sessionID)
}

func (a *Agent) persistApproval(ctx context.Context, sessionID, key string) {
	as, ok := a.store.(store.ApprovalStore)
	if !ok {
		return
	}
	_ = as.GrantApproval(ctx, sessionID, key)
}
