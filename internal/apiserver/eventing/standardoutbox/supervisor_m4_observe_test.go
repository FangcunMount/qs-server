package standardoutbox

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	"github.com/FangcunMount/reliable-messaging/transport"
)

type deliveryMetricObserver struct {
	eventobservability.NopObserver
	events []eventobservability.OutboxEvent
}

func (o *deliveryMetricObserver) ObserveOutbox(_ context.Context, event eventobservability.OutboxEvent) {
	o.events = append(o.events, event)
}

type deliveryMetricStore struct {
	claim     outbox.Claim
	claimed   bool
	scanErr   error
	writeErr  error
	settled   string
	lastClaim outbox.Claim
	lastDelay time.Duration
	lastCode  string
	cancel    context.CancelFunc
}

func (s *deliveryMetricStore) ClaimDue(context.Context, int, time.Duration) ([]outbox.Claim, error) {
	if s.scanErr != nil {
		return []outbox.Claim{s.claim}, s.scanErr
	}
	if s.claimed {
		return nil, nil
	}
	s.claimed = true
	return []outbox.Claim{s.claim}, nil
}

func (s *deliveryMetricStore) Confirm(_ context.Context, claim outbox.Claim) error {
	return s.write("confirm", claim, 0, "")
}

func (s *deliveryMetricStore) Retry(_ context.Context, claim outbox.Claim, delay time.Duration, code string) error {
	return s.write("retry", claim, delay, code)
}

func (s *deliveryMetricStore) Quarantine(_ context.Context, claim outbox.Claim, code string) error {
	return s.write("quarantine", claim, 0, code)
}

func (s *deliveryMetricStore) write(kind string, claim outbox.Claim, delay time.Duration, code string) error {
	s.settled, s.lastClaim, s.lastDelay, s.lastCode = kind, claim, delay, code
	if s.cancel != nil {
		s.cancel()
	}
	return s.writeErr
}

type deliveryMetricPublisher struct {
	result transport.Result
	got    message.Message
}

func (p *deliveryMetricPublisher) Publish(_ context.Context, msg message.Message) transport.Result {
	p.got = msg
	return p.result
}

func metricProofMessage(t *testing.T) message.Message {
	t.Helper()
	msg, err := message.New(message.Input{
		Producer: "qs-server", ID: "original-event", Destination: "qs.evaluation.lifecycle",
		EventType: "answersheet.submitted", SchemaVersion: "v1", Scope: "org:1",
		ContentType: "application/json", OccurredAt: "2026-10-07T10:00:00+08:00", Payload: []byte("original-wire"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestObservedRelayCountsDurableSettlementRatherThanPUBConfirmation(t *testing.T) {
	writeErr := errors.New("injected settlement failure")
	for _, tc := range []struct {
		name       string
		outcome    transport.Outcome
		quarantine bool
		writeErr   error
		settled    string
		want       []eventobservability.OutboxOutcome
	}{
		{"confirmed", transport.Confirmed, false, nil, "confirm", []eventobservability.OutboxOutcome{eventobservability.OutboxOutcomePublished}},
		{"confirmed-write-failed", transport.Confirmed, false, writeErr, "confirm", []eventobservability.OutboxOutcome{eventobservability.OutboxOutcomeMarkPublishedFailed}},
		{"unknown-retry", transport.Unknown, false, nil, "retry", []eventobservability.OutboxOutcome{eventobservability.OutboxOutcomePublishFailed}},
		{"unknown-retry-write-failed", transport.Unknown, false, writeErr, "retry", []eventobservability.OutboxOutcome{eventobservability.OutboxOutcomePublishFailed, eventobservability.OutboxOutcomeMarkFailedFailed}},
		{"rejected-quarantine", transport.Rejected, true, nil, "quarantine", []eventobservability.OutboxOutcome{eventobservability.OutboxOutcomePublishFailed}},
		{"rejected-quarantine-write-failed", transport.Rejected, true, writeErr, "quarantine", []eventobservability.OutboxOutcome{eventobservability.OutboxOutcomePublishFailed, eventobservability.OutboxOutcomeMarkFailedFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := metricProofMessage(t)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			claim := outbox.Claim{RecordID: "opaque-record", Token: "original-token", Version: 7, Attempts: 4, FailureCount: 2, Message: msg}
			store := &deliveryMetricStore{claim: claim, writeErr: tc.writeErr, cancel: cancel}
			publisher := &deliveryMetricPublisher{result: transport.Result{Outcome: tc.outcome}}
			observer := &deliveryMetricObserver{}
			observedStore, observedPublisher := ObserveRelayDelivery(store, publisher, "mongo-domain-events", observer)
			var policyOutcome transport.Outcome
			runner, err := relay.New(observedStore, observedPublisher, relay.Config{
				Concurrency: 1, PollInterval: time.Second, Lease: time.Second,
				PublishTimeout: 100 * time.Millisecond, WriteTimeout: 100 * time.Millisecond,
				Observe: func(relay.Event) {},
				Retry: func(_ outbox.Claim, outcome transport.Outcome) relay.RetryDecision {
					policyOutcome = outcome
					return relay.RetryDecision{Delay: time.Second, Quarantine: tc.quarantine}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := runner.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if store.settled != tc.settled || !reflect.DeepEqual(store.lastClaim, claim) || publisher.got.Fingerprint() != msg.Fingerprint() {
				t.Fatalf("original delivery changed: store=%+v publisher=%+v", store, publisher)
			}
			if tc.outcome != transport.Confirmed && policyOutcome != tc.outcome {
				t.Fatalf("retry outcome changed from %v to %v", tc.outcome, policyOutcome)
			}
			if tc.settled == "retry" && (store.lastDelay != time.Second || store.lastCode != "publish_unknown") {
				t.Fatalf("retry scheduling changed: %+v", store)
			}
			var got []eventobservability.OutboxOutcome
			for _, event := range observer.events {
				if event.Relay != "mongo-domain-events" || event.Topic != msg.Input().Destination || event.EventType != msg.Input().EventType || event.AttemptClass != "" {
					t.Fatalf("fabricated metric identity or attempt: %+v", event)
				}
				got = append(got, event.Outcome)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("outcomes=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestObservedSDKCallsReturnOriginalErrorsAndClaims(t *testing.T) {
	original := errors.New("original error")
	claim := outbox.Claim{RecordID: "opaque", Token: "token", Version: 9, Message: metricProofMessage(t)}
	store := &deliveryMetricStore{claim: claim, scanErr: original, writeErr: original}
	observer := &deliveryMetricObserver{}
	wrapped, _ := ObserveRelayDelivery(store, &deliveryMetricPublisher{}, "assessment-mysql-outbox", observer)
	claims, err := wrapped.ClaimDue(t.Context(), 3, time.Minute)
	if err != original || !reflect.DeepEqual(claims, []outbox.Claim{claim}) {
		t.Fatalf("scan result changed: claims=%+v error=%v", claims, err)
	}
	if event := observer.events[0]; event.Outcome != eventobservability.OutboxOutcomeClaimFailed || event.Topic != "" || event.EventType != "" {
		t.Fatalf("scan invented message identity: %+v", event)
	}
	if err := wrapped.Confirm(t.Context(), claim); err != original {
		t.Fatalf("confirm error changed: %v", err)
	}
	if err := wrapped.Retry(t.Context(), claim, time.Minute, "publish_unknown"); err != original || store.lastDelay != time.Minute || store.lastCode != "publish_unknown" {
		t.Fatalf("retry changed: store=%+v error=%v", store, err)
	}
	if err := wrapped.Quarantine(t.Context(), claim, "publish_rejected"); err != original || store.lastCode != "publish_rejected" {
		t.Fatalf("quarantine changed: store=%+v error=%v", store, err)
	}
}

func TestObservedPublisherPreservesEveryTransportOutcome(t *testing.T) {
	for _, outcome := range []transport.Outcome{transport.Confirmed, transport.Unknown, transport.Rejected} {
		msg := metricProofMessage(t)
		original := transport.Result{Outcome: outcome}
		publisher := &deliveryMetricPublisher{result: original}
		observer := &deliveryMetricObserver{}
		_, wrapped := ObserveRelayDelivery(nil, publisher, "mongo-domain-events", observer)
		if got := wrapped.Publish(t.Context(), msg); got != original || !reflect.DeepEqual(publisher.got, msg) {
			t.Fatalf("publisher result or message changed: outcome=%v message=%+v", got, publisher.got)
		}
		if outcome == transport.Confirmed && len(observer.events) != 0 {
			t.Fatalf("PUB confirmation counted before durable settlement: %+v", observer.events)
		}
		if outcome != transport.Confirmed && (len(observer.events) != 1 || observer.events[0].Outcome != eventobservability.OutboxOutcomePublishFailed) {
			t.Fatalf("non-confirmed PUB observation=%+v", observer.events)
		}
	}
	if store, publisher := ObserveRelayDelivery(nil, nil, "unused", nil); store != nil || publisher != nil {
		t.Fatal("absent SDK dependencies were hidden by wrappers")
	}
}
