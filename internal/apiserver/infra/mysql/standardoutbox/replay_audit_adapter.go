package standardoutbox

import (
	"context"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/outboxreplay"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

func (l *ReplayLedger) ResolvePending(ctx context.Context, audit app.ActionAuditRecord, input app.ReplayPendingInput) ([]outboxport.ManualReplayResult, bool, error) {
	return outboxreplay.ResolvePending(ctx, l, audit, input)
}

func (l *ReplayLedger) AuthorizeManualReplayWithReason(ctx context.Context, orgID int64, requestID, reason string, targets []outboxport.ManualReplayTarget) ([]outboxport.ManualReplayResult, error) {
	return outboxreplay.AuthorizeManualReplayWithReason(ctx, l, l.storeName, orgID, requestID, reason, targets)
}

var _ outboxreplay.Ledger = (*ReplayLedger)(nil)
var _ app.PendingReplayResolver = (*ReplayLedger)(nil)
var _ outboxport.DurableManualReplayAuthorizer = (*ReplayLedger)(nil)
