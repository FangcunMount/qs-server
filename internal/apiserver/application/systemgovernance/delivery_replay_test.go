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

func TestActionExecutorDoesNotReplayLegacyTaskOpenedNotification(t *testing.T) {
	evt := event.New("task.opened", "AssessmentTask", "42", map[string]any{"org_id": int64(9)})
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeDeliveryReplayStore{authorized: []AuthorizedDelivery{{ID: 7, EventID: evt.EventID(), PayloadJSON: string(payload)}}}
	publisher := &fakeDeliveryPublisher{}
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{}).BindDeliveryReplay(store, publisher)

	_, err = executor.Run(t.Context(), 9, "events.replay_delivery", singleDeliveryReplayRequest("legacy-task-opened"))
	if err == nil || !strings.Contains(err.Error(), "人工核对") {
		t.Fatalf("legacy task.opened must require reconciliation: %v", err)
	}
	if publisher.publishCount != 0 || store.failedID != 7 || store.completedID != 0 {
		t.Fatalf("legacy task.opened must not publish: publisher=%#v store=%#v", publisher, store)
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

func TestActionExecutorDeliveryReplayKeepsClaimWhenCompletionWriteFails(t *testing.T) {
	evt := event.New("evaluation.retry.requested", "Evaluation", "42", map[string]any{"org_id": int64(9)})
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeDeliveryReplayStore{
		authorized:  []AuthorizedDelivery{{ID: 7, EventID: evt.EventID(), PayloadJSON: string(payload)}},
		completeErr: fmt.Errorf("completion write unavailable"),
	}
	publisher := &fakeDeliveryPublisher{}
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{}).BindDeliveryReplay(store, publisher)
	_, err = executor.Run(t.Context(), 9, "events.replay_delivery", singleDeliveryReplayRequest("completion-write-failed"))
	if err == nil || !strings.Contains(err.Error(), "完成状态待核对") {
		t.Fatalf("completion write failure must remain uncertain: %v", err)
	}
	if publisher.publishCount != 1 || store.completedID != 7 || store.uncertainID != 7 || store.failedID != 0 {
		t.Fatalf("published=%d completion=%d uncertain=%d failed=%d", publisher.publishCount, store.completedID, store.uncertainID, store.failedID)
	}
}

func TestActionExecutorDeliveryReplayExposesFailureStateWriteError(t *testing.T) {
	store := &fakeDeliveryReplayStore{
		authorized: []AuthorizedDelivery{{ID: 7, EventID: "bad-event", PayloadJSON: "{"}},
		failErr:    fmt.Errorf("failure state write unavailable"),
	}
	publisher := &fakeDeliveryPublisher{}
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{}).BindDeliveryReplay(store, publisher)
	_, err := executor.Run(t.Context(), 9, "events.replay_delivery", singleDeliveryReplayRequest("failure-write-failed"))
	if err == nil || !strings.Contains(err.Error(), "状态待核对") {
		t.Fatalf("failed compensation must remain visible: %v", err)
	}
	if publisher.publishCount != 0 || store.failedID != 7 || store.uncertainID != 0 {
		t.Fatalf("published=%d failure_attempt=%d uncertain=%d", publisher.publishCount, store.failedID, store.uncertainID)
	}
}

func TestActionExecutorDeliveryReplaySettlesAfterCallerCancellation(t *testing.T) {
	evt := event.New("evaluation.retry.requested", "Evaluation", "42", map[string]any{"org_id": int64(9)})
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeDeliveryReplayStore{authorized: []AuthorizedDelivery{{ID: 7, EventID: evt.EventID(), PayloadJSON: string(payload)}}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	publisher := &fakeDeliveryPublisher{afterPublish: cancel}
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{}).BindDeliveryReplay(store, publisher)
	result, err := executor.Run(ctx, 9, "events.replay_delivery", singleDeliveryReplayRequest("caller-canceled"))
	if err != nil || result == nil || result.Status != "ok" || store.completedID != 7 {
		t.Fatalf("published event must settle after caller cancellation: result=%#v err=%v completion=%d", result, err, store.completedID)
	}
}

func singleDeliveryReplayRequest(requestID string) ActionRunRequest {
	return ActionRunRequest{
		RequestID: requestID,
		Confirm:   true,
		Input: map[string]interface{}{
			"reason":  "transport recovered",
			"targets": []interface{}{map[string]interface{}{"id": 7, "expected_delivery_attempts": 8}},
		},
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
	completeErr error
	failErr     error
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
func (s *fakeDeliveryReplayStore) CompleteReplay(ctx context.Context, id uint64, _ string, _ time.Time) error {
	s.completedID = id
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.completeErr
}
func (s *fakeDeliveryReplayStore) FailReplay(ctx context.Context, id uint64, _, _ string, _ time.Time) error {
	s.failedID = id
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.failErr
}

func (s *fakeDeliveryReplayStore) RecordReplayUncertain(ctx context.Context, id uint64, _, _ string, _ time.Time) error {
	s.uncertainID = id
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

type fakeDeliveryPublisher struct {
	eventID      string
	publishCount int
	failOn       int
	afterPublish func()
}

func (p *fakeDeliveryPublisher) Publish(_ context.Context, evt event.DomainEvent) error {
	p.publishCount++
	p.eventID = evt.EventID()
	if p.afterPublish != nil {
		p.afterPublish()
	}
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
