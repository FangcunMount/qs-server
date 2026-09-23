package systemgovernance

import (
	"context"
	"errors"
	"testing"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

type fixedRunningAudit struct {
	record ActionAuditRecord
	found  bool
	err    error
	marks  *int
}

func (r fixedRunningAudit) LoadRunning(context.Context, ActionAuditRecord) (ActionAuditRecord, bool, error) {
	return r.record, r.found, r.err
}

func (r fixedRunningAudit) MarkPending(context.Context, ActionAuditRecord) error {
	if r.marks != nil {
		*r.marks++
	}
	return nil
}

type fixedPendingResolver struct {
	results []outboxport.ManualReplayResult
	found   bool
	err     error
	calls   int
}

func (r *fixedPendingResolver) ResolvePending(_ context.Context, _ ActionAuditRecord, _ ReplayPendingInput) ([]outboxport.ManualReplayResult, bool, error) {
	r.calls++
	return r.results, r.found, r.err
}

func TestRunningReplayAuditRecoversOnlyCommittedMatchingResult(t *testing.T) {
	input := map[string]interface{}{
		"store": "assessment-mysql-outbox", "reason": "reviewed",
		"targets": []interface{}{map[string]interface{}{"event_id": "event-1", "expected_attempt_count": 30}},
	}
	started := time.Date(2026, 9, 23, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	stored := ActionAuditRecord{
		RequestID: "request-1", ActionID: "events.replay_pending", OrgID: 7,
		Input: input, StartedAt: started, Status: "running",
	}
	base := &memoryActionAudit{running: true, results: map[string]*ActionAuditReplay{}}
	resolver := &fixedPendingResolver{
		found: true, results: []outboxport.ManualReplayResult{{EventID: "event-1", Authorized: true}},
	}
	audit := NewReconcilingActionAuditStore(base, fixedRunningAudit{record: stored, found: true},
		map[string]PendingReplayResolver{"assessment-mysql-outbox": resolver})
	executor := NewActionExecutorWithResilience(NewActionRegistry(), &fakeStatisticsGovernance{}, nil, nil, audit)
	req := ActionRunRequest{RequestID: "request-1", Confirm: true, Input: input}
	first, err := executor.Run(context.Background(), 7, "events.replay_pending", req)
	if err != nil || first == nil || first.RequestID != req.RequestID || !first.StartedAt.Equal(started) ||
		first.Result["authorized"] != 1 || base.completes != 1 || resolver.calls != 1 {
		t.Fatalf("running audit not recovered: result=%+v err=%v completes=%d resolves=%d", first, err, base.completes, resolver.calls)
	}
	second, err := executor.Run(context.Background(), 7, "events.replay_pending", req)
	if err != nil || second == nil || second.RequestID != first.RequestID || base.completes != 1 || resolver.calls != 1 {
		t.Fatalf("completed audit was reauthorized: result=%+v err=%v completes=%d resolves=%d", second, err, base.completes, resolver.calls)
	}
}

func TestRunningReplayAuditWithoutDurableResultStaysPending(t *testing.T) {
	stored := ActionAuditRecord{
		RequestID: "request-missing", ActionID: "events.replay_pending", OrgID: 7,
		Input: map[string]interface{}{
			"store": "assessment-mysql-outbox", "reason": "reviewed",
			"targets": []interface{}{map[string]interface{}{"event_id": "event-1", "expected_attempt_count": 30}},
		}, StartedAt: time.Now(), Status: "running",
	}
	base := &memoryActionAudit{running: true, results: map[string]*ActionAuditReplay{}}
	resolver := &fixedPendingResolver{found: false}
	marks := 0
	audit := NewReconcilingActionAuditStore(base, fixedRunningAudit{record: stored, found: true, marks: &marks},
		map[string]PendingReplayResolver{"assessment-mysql-outbox": resolver})
	prior, claimed, err := audit.Claim(context.Background(), stored)
	if !errors.Is(err, ErrActionAuditPendingReconciliation) || prior != nil || claimed || base.completes != 0 || resolver.calls != 1 || marks != 1 {
		t.Fatalf("missing ledger incorrectly completed audit: prior=%+v claimed=%t err=%v", prior, claimed, err)
	}
	resolver.found = true
	resolver.results = []outboxport.ManualReplayResult{{EventID: "wrong-event", Authorized: true}}
	if _, _, err := audit.Claim(context.Background(), stored); err == nil || base.completes != 0 {
		t.Fatalf("mismatched ledger incorrectly completed audit: err=%v", err)
	}
	readerFailure := errors.New("stored input mismatch")
	audit = NewReconcilingActionAuditStore(base, fixedRunningAudit{err: readerFailure},
		map[string]PendingReplayResolver{"assessment-mysql-outbox": resolver})
	if _, _, err := audit.Claim(context.Background(), stored); !errors.Is(err, readerFailure) || base.completes != 0 {
		t.Fatalf("mismatched running audit was reused: err=%v", err)
	}
}
