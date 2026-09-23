package systemgovernance

import (
	"context"
	"errors"
	"testing"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type recordingDurableReplay struct {
	orgID     int64
	requestID string
	reason    string
	targets   []outboxport.ManualReplayTarget
	calls     int
	err       error
}

func (r *recordingDurableReplay) AuthorizeManualReplayWithReason(_ context.Context, orgID int64, requestID, reason string, targets []outboxport.ManualReplayTarget) ([]outboxport.ManualReplayResult, error) {
	r.orgID, r.requestID, r.reason, r.targets = orgID, requestID, reason, targets
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return []outboxport.ManualReplayResult{{EventID: targets[0].EventID, Authorized: true}}, nil
}

type recordingLegacyReplay struct{ calls int }

func (r *recordingLegacyReplay) AuthorizeManualReplay(_ context.Context, _ int64, _ string, _ []outboxport.ManualReplayTarget, _ time.Time) ([]outboxport.ManualReplayResult, error) {
	r.calls++
	return nil, nil
}

func TestReplayPendingPassesApprovalReasonToDurableOwner(t *testing.T) {
	store := &recordingDurableReplay{}
	executor := NewActionExecutor(NewActionRegistry(), nil).BindDurableEventReplayStores(
		map[string]outboxport.DurableManualReplayAuthorizer{"assessment-mysql-outbox": store})
	result, err := executor.Run(context.Background(), 7, "events.replay_pending", ActionRunRequest{
		RequestID: "approval-1", Confirm: true,
		Input: map[string]interface{}{
			"store": "assessment-mysql-outbox", "reason": "reviewed by operator",
			"targets": []interface{}{map[string]interface{}{"event_id": "event-1", "expected_attempt_count": 30}},
		},
	})
	if err != nil || result == nil || result.Result["authorized"] != 1 || store.calls != 1 ||
		store.orgID != 7 || store.requestID != "approval-1" || store.reason != "reviewed by operator" ||
		len(store.targets) != 1 || store.targets[0].EventID != "event-1" || store.targets[0].ExpectedAttemptCount != 30 {
		t.Fatalf("durable replay lost approval input: result=%+v err=%v owner=%+v", result, err, store)
	}
}

func TestReplayPendingRefusesTwoOwnersForSameProfile(t *testing.T) {
	durable := &recordingDurableReplay{}
	legacy := &recordingLegacyReplay{}
	executor := NewActionExecutor(NewActionRegistry(), nil).
		BindDurableEventReplayStores(map[string]outboxport.DurableManualReplayAuthorizer{"assessment-mysql-outbox": durable}).
		BindEventReplayStores(map[string]outboxport.ManualReplayAuthorizer{"assessment-mysql-outbox": legacy})
	_, err := executor.Run(context.Background(), 7, "events.replay_pending", ActionRunRequest{
		RequestID: "approval-2", Confirm: true,
		Input: map[string]interface{}{
			"store": "assessment-mysql-outbox", "reason": "reviewed",
			"targets": []interface{}{map[string]interface{}{"event_id": "event-1", "expected_attempt_count": 30}},
		},
	})
	if err == nil || baseerrors.ParseCoder(err).Code() != code.ErrInternalServerError || durable.calls != 0 || legacy.calls != 0 {
		t.Fatalf("two owners executed or were accepted: err=%v durable=%d legacy=%d", err, durable.calls, legacy.calls)
	}
}

func TestReplayPendingUnknownCommitKeepsAuditOpen(t *testing.T) {
	store := &recordingDurableReplay{err: outboxport.ErrManualReplayOutcomeUnknown}
	audit := &memoryActionAudit{results: map[string]*ActionAuditReplay{}}
	executor := NewActionExecutorWithResilience(NewActionRegistry(), nil, nil, nil, audit).
		BindDurableEventReplayStores(map[string]outboxport.DurableManualReplayAuthorizer{"assessment-mysql-outbox": store})
	req := ActionRunRequest{
		RequestID: "unknown-commit", Confirm: true,
		Input: map[string]interface{}{
			"store": "assessment-mysql-outbox", "reason": "reviewed",
			"targets": []interface{}{map[string]interface{}{"event_id": "event-1", "expected_attempt_count": 30}},
		},
	}
	if _, err := executor.Run(context.Background(), 7, "events.replay_pending", req); !errors.Is(err, outboxport.ErrManualReplayOutcomeUnknown) {
		t.Fatalf("unknown outcome was hidden: %v", err)
	}
	if audit.completes != 0 || !audit.running || store.calls != 1 {
		t.Fatalf("unknown outcome was finalized or retried: completes=%d running=%t calls=%d", audit.completes, audit.running, store.calls)
	}
	if _, err := executor.Run(context.Background(), 7, "events.replay_pending", req); err == nil || store.calls != 1 || audit.completes != 0 {
		t.Fatalf("same request blindly reauthorized: err=%v calls=%d completes=%d", err, store.calls, audit.completes)
	}
}
