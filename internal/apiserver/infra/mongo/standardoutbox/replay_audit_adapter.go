//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func (l *ReplayLedger) ResolvePending(ctx context.Context, audit app.ActionAuditRecord, input app.ReplayPendingInput) ([]outboxport.ManualReplayResult, bool, error) {
	if audit.ActionID != "events.replay_pending" {
		return nil, false, errors.New("wrong governance action for Outbox replay")
	}
	replay, err := durableReplayRequest(audit.OrgID, audit.RequestID, input.Store, input.Reason, input.Targets)
	if err != nil {
		return nil, false, err
	}
	items, found, err := l.Resolve(ctx, replay)
	if err != nil || !found {
		return nil, found, err
	}
	return manualReplayResults(items), true, nil
}

func (l *ReplayLedger) AuthorizeManualReplayWithReason(ctx context.Context, orgID int64, requestID, reason string, targets []outboxport.ManualReplayTarget) ([]outboxport.ManualReplayResult, error) {
	replay, err := durableReplayRequest(orgID, requestID, l.storeName, reason, targets)
	if err != nil {
		return nil, baseerrors.WithCode(code.ErrInvalidArgument, "invalid durable replay request: %s", err.Error())
	}
	items, err := l.Authorize(ctx, replay)
	if errors.Is(err, request.ErrReplayInputConflict) {
		return nil, baseerrors.WithCode(code.ErrConflict, "request_id already belongs to different replay input")
	}
	if err != nil {
		resolveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		prior, found, resolveErr := l.Resolve(resolveCtx, replay)
		if resolveErr == nil && found {
			return manualReplayResults(prior), nil
		}
		return nil, fmt.Errorf("%w: authorize: %v; resolve: %v", outboxport.ErrManualReplayOutcomeUnknown, err, resolveErr)
	}
	return manualReplayResults(items), nil
}

func durableReplayRequest(orgID int64, requestID, store, reason string, targets []outboxport.ManualReplayTarget) (request.ReplayRequest, error) {
	converted := make([]request.ReplayTarget, len(targets))
	for i, target := range targets {
		if target.ExpectedAttemptCount <= 0 {
			return request.ReplayRequest{}, errors.New("positive expected failure count required")
		}
		converted[i] = request.ReplayTarget{EventID: target.EventID, ExpectedFailureCount: uint64(target.ExpectedAttemptCount)}
	}
	replay := request.ReplayRequest{
		OrgID: orgID, RequestID: requestID, Store: store, Reason: reason, Targets: converted,
	}
	if _, err := replay.Fingerprint(); err != nil {
		return request.ReplayRequest{}, err
	}
	return replay, nil
}

func manualReplayResults(items []request.ReplayResult) []outboxport.ManualReplayResult {
	results := make([]outboxport.ManualReplayResult, len(items))
	for i, item := range items {
		results[i] = outboxport.ManualReplayResult{EventID: item.EventID, Authorized: item.Authorized, Reason: item.Reason}
	}
	return results
}

var _ app.PendingReplayResolver = (*ReplayLedger)(nil)
var _ outboxport.DurableManualReplayAuthorizer = (*ReplayLedger)(nil)
