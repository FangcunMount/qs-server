package platform

import (
	"context"
	"testing"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

type statusOnlyDurableReplay struct{ calls int }

func (*statusOnlyDurableReplay) OutboxStatusSnapshot(context.Context, time.Time) (outboxport.StatusSnapshot, error) {
	return outboxport.StatusSnapshot{}, nil
}

func (r *statusOnlyDurableReplay) AuthorizeManualReplayWithReason(context.Context, int64, string, string, []outboxport.ManualReplayTarget) ([]outboxport.ManualReplayResult, error) {
	r.calls++
	return nil, nil
}

type recoverableStatusReplay struct{ *statusOnlyDurableReplay }

func (*recoverableStatusReplay) ResolvePending(context.Context, systemgov.ActionAuditRecord, systemgov.ReplayPendingInput) ([]outboxport.ManualReplayResult, bool, error) {
	return nil, false, nil
}

func TestStandardReplayRequiresRecoveryAndPersistentAudit(t *testing.T) {
	incomplete := &statusOnlyDurableReplay{}
	complete := &recoverableStatusReplay{statusOnlyDurableReplay: &statusOnlyDurableReplay{}}
	stores, resolvers := buildDurableEventReplayStores([]appEventing.NamedOutboxStatusReader{
		{Name: "incomplete", Reader: incomplete},
		{Name: "standard", Reader: complete},
	})
	if len(stores) != 1 || len(resolvers) != 1 || stores["standard"] != complete || resolvers["standard"] != complete {
		t.Fatalf("durable replay owner and recovery resolver diverged: stores=%v resolvers=%v", stores, resolvers)
	}
	facade := BuildRESTSystemGovernanceFacade(RESTSystemGovernanceInput{
		EventOutboxes: []appEventing.NamedOutboxStatusReader{{Name: "standard", Reader: complete}},
	})
	_, err := facade.RunAction(context.Background(), 7, "events.replay_pending", systemgov.ActionRunRequest{
		RequestID: "no-audit", Confirm: true,
		Input: map[string]interface{}{
			"store": "standard", "reason": "reviewed",
			"targets": []interface{}{map[string]interface{}{"event_id": "one", "expected_attempt_count": 30}},
		},
	})
	if err == nil || complete.calls != 0 {
		t.Fatalf("standard replay ran without a persistent audit: err=%v calls=%d", err, complete.calls)
	}
}
