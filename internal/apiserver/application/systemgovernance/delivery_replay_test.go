package systemgovernance

import (
	"context"
	"encoding/json"
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

type fakeDeliveryReplayStore struct {
	authorized  []AuthorizedDelivery
	orgID       int64
	requestID   string
	targets     []DeliveryReplayTarget
	completedID uint64
	failedID    uint64
}

func (s *fakeDeliveryReplayStore) AuthorizeReplay(_ context.Context, orgID int64, requestID string, targets []DeliveryReplayTarget, _ time.Time) ([]AuthorizedDelivery, error) {
	s.orgID, s.requestID, s.targets = orgID, requestID, append([]DeliveryReplayTarget(nil), targets...)
	return append([]AuthorizedDelivery(nil), s.authorized...), nil
}
func (s *fakeDeliveryReplayStore) CompleteReplay(_ context.Context, id uint64, _ string, _ time.Time) error {
	s.completedID = id
	return nil
}
func (s *fakeDeliveryReplayStore) FailReplay(_ context.Context, id uint64, _, _ string, _ time.Time) error {
	s.failedID = id
	return nil
}

type fakeDeliveryPublisher struct{ eventID string }

func (p *fakeDeliveryPublisher) Publish(_ context.Context, evt event.DomainEvent) error {
	p.eventID = evt.EventID()
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
