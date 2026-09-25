package systemgovernance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
)

func TestActionExecutorReplaysTransportDeadLetterWithOriginalEventID(t *testing.T) {
	evt := event.New("evaluation.retry.requested", "Evaluation", "42", map[string]any{"org_id": int64(9)})
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeDeliveryReplayStore{authorized: []AuthorizedDelivery{{ID: 7, EventID: evt.EventID(), PayloadJSON: string(payload)}}}
	publisher := &fakeDeliveryPublisher{}
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{}).BindDeliveryReplay(store, publisher)

	result, err := executor.Run(t.Context(), 9, "events.replay_delivery", ActionRunRequest{
		RequestID: "delivery-replay-1", Confirm: true,
		Input: map[string]interface{}{
			"reason":  "transport recovered",
			"targets": []interface{}{map[string]interface{}{"id": 7, "expected_delivery_attempts": 8}},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != "ok" || publisher.eventID != evt.EventID() || store.completedID != 7 || store.failedID != 0 {
		t.Fatalf("result=%#v publisher=%#v store=%#v", result, publisher, store)
	}
	if store.orgID != 9 || store.requestID != "delivery-replay-1" || len(store.targets) != 1 || store.targets[0].ExpectedDeliveryAttempts != 8 {
		t.Fatalf("authorization scope = %#v", store)
	}
}

func TestActionExecutorDeliveryReplayLeavesUnattemptedBatchItemsManualOnUnknownPublish(t *testing.T) {
	store := &fakeDeliveryReplayStore{}
	targets := make([]interface{}, 0, 3)
	for id := uint64(1); id <= 3; id++ {
		evt := event.New("evaluation.retry.requested", "Evaluation", fmt.Sprint(id), map[string]any{"org_id": int64(9)})
		payload, err := json.Marshal(evt)
		if err != nil {
			t.Fatal(err)
		}
		store.authorized = append(store.authorized, AuthorizedDelivery{ID: id, EventID: evt.EventID(), PayloadJSON: string(payload)})
		targets = append(targets, map[string]interface{}{"id": id, "expected_delivery_attempts": 8})
	}
	publisher := &fakeDeliveryPublisher{failOn: 2}
	audit := &memoryActionAudit{results: map[string]*ActionAuditReplay{}}
	executor := NewActionExecutorWithResilience(NewActionRegistry(), &fakeStatisticsGovernance{}, nil, nil, audit).BindDeliveryReplay(store, publisher)
	request := ActionRunRequest{
		RequestID: "delivery-batch", Confirm: true,
		Input: map[string]interface{}{"reason": "transport recovered", "targets": targets},
	}
	_, err := executor.Run(t.Context(), 9, "events.replay_delivery", request)
	if err == nil || !strings.Contains(err.Error(), "已完成 1 条") || !strings.Contains(err.Error(), "待核对") {
		t.Fatalf("second publish should report partial completion and require reconciliation: %v", err)
	}
	if len(store.claims) != 2 || store.claims[0] != 1 || store.claims[1] != 2 {
		t.Fatalf("claimed IDs = %v; item 3 must remain unclaimed", store.claims)
	}
	if store.completedID != 1 || store.uncertainID != 2 || store.failedID != 0 || publisher.publishCount != 2 {
		t.Fatalf("completion=%d uncertain=%d failed=%d published=%d", store.completedID, store.uncertainID, store.failedID, publisher.publishCount)
	}
	if prior := audit.results[request.RequestID]; prior == nil || prior.Error == nil || prior.Error.Message != err.Error() {
		t.Fatalf("audit must preserve actionable partial result: %#v", prior)
	}
	_, replayErr := executor.Run(t.Context(), 9, "events.replay_delivery", request)
	if replayErr == nil || replayErr.Error() != err.Error() || publisher.publishCount != 2 {
		t.Fatalf("same request must return prior reconciliation result without publishing: err=%v publish_count=%d", replayErr, publisher.publishCount)
	}
}

func TestActionExecutorDeliveryReplayRejectsDuplicateBeforeClaim(t *testing.T) {
	store := &fakeDeliveryReplayStore{}
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{}).BindDeliveryReplay(store, &fakeDeliveryPublisher{})
	_, err := executor.Run(t.Context(), 9, "events.replay_delivery", ActionRunRequest{
		RequestID: "delivery-duplicate", Confirm: true,
		Input: map[string]interface{}{"reason": "transport recovered", "targets": []interface{}{
			map[string]interface{}{"id": 7, "expected_delivery_attempts": 8},
			map[string]interface{}{"id": 7, "expected_delivery_attempts": 8},
		}},
	})
	if err == nil || len(store.claims) != 0 {
		t.Fatalf("duplicate should be rejected before claim: err=%v claims=%v", err, store.claims)
	}
}

func TestActionExecutorDeliveryReplayPrevalidatesWholeBatch(t *testing.T) {
	store := &fakeDeliveryReplayStore{validateErr: fmt.Errorf("delivery 2 outside organization scope")}
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{}).BindDeliveryReplay(store, &fakeDeliveryPublisher{})
	_, err := executor.Run(t.Context(), 9, "events.replay_delivery", ActionRunRequest{
		RequestID: "delivery-foreign", Confirm: true,
		Input: map[string]interface{}{"reason": "transport recovered", "targets": []interface{}{
			map[string]interface{}{"id": 1, "expected_delivery_attempts": 8},
			map[string]interface{}{"id": 2, "expected_delivery_attempts": 8},
		}},
	})
	if err == nil || len(store.claims) != 0 {
		t.Fatalf("invalid later target must prevent earlier publish: err=%v claims=%v", err, store.claims)
	}
}

type fakeDeliveryReplayStore struct {
	authorized  []AuthorizedDelivery
	orgID       int64
	requestID   string
	targets     []DeliveryReplayTarget
	completedID uint64
	failedID    uint64
	uncertainID uint64
	claims      []uint64
	validateErr error
}

func (s *fakeDeliveryReplayStore) ValidateReplayBatch(_ context.Context, _ int64, _ []DeliveryReplayTarget) error {
	return s.validateErr
}

func (s *fakeDeliveryReplayStore) AuthorizeReplay(_ context.Context, orgID int64, requestID string, targets []DeliveryReplayTarget, _ time.Time) ([]AuthorizedDelivery, error) {
	s.orgID, s.requestID, s.targets = orgID, requestID, append([]DeliveryReplayTarget(nil), targets...)
	if len(targets) != 1 {
		return nil, fmt.Errorf("expected one claim, got %d", len(targets))
	}
	s.claims = append(s.claims, targets[0].ID)
	for _, item := range s.authorized {
		if item.ID == targets[0].ID {
			return []AuthorizedDelivery{item}, nil
		}
	}
	return nil, fmt.Errorf("delivery %d missing", targets[0].ID)
}
func (s *fakeDeliveryReplayStore) CompleteReplay(_ context.Context, id uint64, _ string, _ time.Time) error {
	s.completedID = id
	return nil
}
func (s *fakeDeliveryReplayStore) FailReplay(_ context.Context, id uint64, _, _ string, _ time.Time) error {
	s.failedID = id
	return nil
}

func (s *fakeDeliveryReplayStore) RecordReplayUncertain(_ context.Context, id uint64, _, _ string, _ time.Time) error {
	s.uncertainID = id
	return nil
}

type fakeDeliveryPublisher struct {
	eventID      string
	publishCount int
	failOn       int
}

func (p *fakeDeliveryPublisher) Publish(_ context.Context, evt event.DomainEvent) error {
	p.publishCount++
	p.eventID = evt.EventID()
	if p.failOn == p.publishCount {
		return fmt.Errorf("publish response lost")
	}
	return nil
}
func (p *fakeDeliveryPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}
