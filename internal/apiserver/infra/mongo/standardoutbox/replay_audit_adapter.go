//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"errors"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

func (l *ReplayLedger) ResolvePending(ctx context.Context, audit app.ActionAuditRecord, input app.ReplayPendingInput) ([]outboxport.ManualReplayResult, bool, error) {
	if audit.ActionID != "events.replay_pending" {
		return nil, false, errors.New("wrong governance action for Outbox replay")
	}
	targets := make([]request.ReplayTarget, len(input.Targets))
	for i, target := range input.Targets {
		if target.ExpectedAttemptCount <= 0 {
			return nil, false, errors.New("positive expected failure count required")
		}
		targets[i] = request.ReplayTarget{EventID: target.EventID, ExpectedFailureCount: uint64(target.ExpectedAttemptCount)}
	}
	items, found, err := l.Resolve(ctx, request.ReplayRequest{
		OrgID: audit.OrgID, RequestID: audit.RequestID, Store: input.Store,
		Reason: input.Reason, Targets: targets,
	})
	if err != nil || !found {
		return nil, found, err
	}
	results := make([]outboxport.ManualReplayResult, len(items))
	for i, item := range items {
		results[i] = outboxport.ManualReplayResult{EventID: item.EventID, Authorized: item.Authorized, Reason: item.Reason}
	}
	return results, true, nil
}

var _ app.PendingReplayResolver = (*ReplayLedger)(nil)
