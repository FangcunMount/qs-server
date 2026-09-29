package eventruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

type capturedWirePublisher struct {
	topic  string
	bodies [][]byte
	err    error
}

func (p *capturedWirePublisher) PublishWire(_ context.Context, topic string, body []byte) error {
	p.topic = topic
	p.bodies = append(p.bodies, append([]byte(nil), body...))
	return p.err
}

func TestRoutingPublisherSDKWireRetainsIdentityOnUnknown(t *testing.T) {
	evt := event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", map[string]string{"id": "sheet-1"})
	wire := &capturedWirePublisher{}
	route := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog: loadEventCatalog(t), WirePublisher: wire,
		Source: "unit-test", Mode: PublishModeMQ,
	})
	if err := route.Publish(t.Context(), evt); err != nil || wire.topic == "" || len(wire.bodies) != 1 {
		t.Fatalf("SDK wire publish: err=%v topic=%q bodies=%d", err, wire.topic, len(wire.bodies))
	}
	envelope, recognized, err := legacy.Decode(wire.bodies[0])
	if err != nil || !recognized || envelope.UUID != evt.EventID() || envelope.Metadata["event_type"] != evt.EventType() || envelope.Metadata["source"] != "unit-test" || len(envelope.Payload) == 0 {
		t.Fatalf("SDK wire identity: envelope=%+v recognized=%t err=%v", envelope, recognized, err)
	}
	wantErr := errors.New("publish confirmation unknown")
	wire.err = wantErr
	if err := route.Publish(t.Context(), evt); !errors.Is(err, wantErr) || len(wire.bodies) != 2 || string(wire.bodies[1]) != string(wire.bodies[0]) {
		t.Fatalf("unknown publish: err=%v bodies=%d identity retained=%t", err, len(wire.bodies), len(wire.bodies) == 2 && string(wire.bodies[1]) == string(wire.bodies[0]))
	}
}

type publishObserver struct {
	events []eventobservability.PublishEvent
}

func (o *publishObserver) ObservePublish(_ context.Context, evt eventobservability.PublishEvent) {
	o.events = append(o.events, evt)
}

func (o *publishObserver) ObserveOutbox(context.Context, eventobservability.OutboxEvent)   {}
func (o *publishObserver) ObserveConsume(context.Context, eventobservability.ConsumeEvent) {}

func TestRoutingPublisherUsesExplicitCatalogAndMetadata(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	mq := &capturedWirePublisher{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:       eventcatalog.NewCatalog(cfg),
		WirePublisher: mq,
		Observer:      &publishObserver{},
		Source:        "unit-test",
		Mode:          PublishModeMQ,
	})
	evt := event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", map[string]string{"id": "sheet-1"})

	if err := publisher.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if mq.topic == "" {
		t.Fatalf("topic was not captured")
	}
	if len(mq.bodies) != 1 {
		t.Fatalf("wire bodies = %d, want 1", len(mq.bodies))
	}
	envelope, recognized, err := legacy.Decode(mq.bodies[0])
	if err != nil || !recognized || envelope.Metadata["event_type"] != eventcatalog.AnswerSheetSubmitted || envelope.Metadata["source"] != "unit-test" || len(envelope.Payload) == 0 {
		t.Fatalf("wire metadata: envelope=%+v recognized=%t err=%v", envelope, recognized, err)
	}
}

func TestRoutingPublisherAllowsDurableOutboxEventForRelayPublish(t *testing.T) {
	catalog := loadEventCatalog(t)
	if !catalog.IsDurableOutbox(eventcatalog.AnswerSheetSubmitted) {
		t.Fatalf("%q must be configured as durable_outbox for this contract test", eventcatalog.AnswerSheetSubmitted)
	}
	mq := &capturedWirePublisher{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:       catalog,
		WirePublisher: mq,
		Source:        "outbox-relay",
		Mode:          PublishModeMQ,
	})
	evt := event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", map[string]string{"id": "sheet-1"})

	if err := publisher.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish durable outbox event from relay path: %v", err)
	}
	if mq.topic == "" {
		t.Fatalf("durable outbox event was not routed to MQ")
	}
	if len(mq.bodies) != 1 {
		t.Fatalf("published wire messages = %d, want 1", len(mq.bodies))
	}
	envelope, recognized, err := legacy.Decode(mq.bodies[0])
	if err != nil || !recognized || envelope.Metadata["event_type"] != eventcatalog.AnswerSheetSubmitted {
		t.Fatalf("published wire metadata: envelope=%+v recognized=%t err=%v", envelope, recognized, err)
	}
}

func TestRoutingPublisherObservesMQPublished(t *testing.T) {
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:       loadEventCatalog(t),
		WirePublisher: &capturedWirePublisher{},
		Observer:      observer,
		Source:        "unit-test",
		Mode:          PublishModeMQ,
	})

	err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", struct{}{}))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeMQPublished)
}

func TestRoutingPublisherObservesNilPublisherFallback(t *testing.T) {
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:  loadEventCatalog(t),
		Observer: observer,
		Source:   "unit-test",
		Mode:     PublishModeMQ,
	})

	err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", struct{}{}))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeFallbackLogged)
}

func TestRoutingPublisherObservesLoggingAndNopModes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    PublishMode
		outcome eventobservability.PublishOutcome
	}{
		{name: "logging", mode: PublishModeLogging, outcome: eventobservability.PublishOutcomeLogged},
		{name: "nop", mode: PublishModeNop, outcome: eventobservability.PublishOutcomeNop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observer := &publishObserver{}
			publisher := NewRoutingPublisher(RoutingPublisherOptions{
				Catalog:  loadEventCatalog(t),
				Observer: observer,
				Source:   "unit-test",
				Mode:     tc.mode,
			})

			err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", struct{}{}))
			if err != nil {
				t.Fatalf("Publish: %v", err)
			}
			assertPublishOutcome(t, observer, tc.outcome)
		})
	}
}

func TestRoutingPublisherObservesUnknownEvent(t *testing.T) {
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog: eventcatalog.NewCatalog(&eventcatalog.Config{
			Topics: map[string]eventcatalog.TopicConfig{},
			Events: map[string]eventcatalog.EventConfig{},
		}),
		Observer: observer,
		Mode:     PublishModeNop,
	})

	err := publisher.Publish(context.Background(), event.New("unknown.event", "Unknown", "1", map[string]string{}))
	if err == nil {
		t.Fatalf("Publish should reject unknown event")
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeUnknownEvent)
}

func TestRoutingPublisherObservesEncodeFailed(t *testing.T) {
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:       loadEventCatalog(t),
		WirePublisher: &capturedWirePublisher{},
		Observer:      observer,
		Mode:          PublishModeMQ,
	})

	err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", map[string]any{
		"bad": func() {},
	}))
	if err == nil {
		t.Fatalf("Publish should fail on non-json payload")
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeEncodeFailed)
}

func TestRoutingPublisherObservesMQFailed(t *testing.T) {
	wantErr := errors.New("mq failed")
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:       loadEventCatalog(t),
		WirePublisher: &capturedWirePublisher{err: wantErr},
		Observer:      observer,
		Mode:          PublishModeMQ,
	})

	err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", struct{}{}))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Publish error = %v, want %v", err, wantErr)
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeMQFailed)
}

func TestRoutingPublisherRejectsUnknownEvent(t *testing.T) {
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog: eventcatalog.NewCatalog(&eventcatalog.Config{
			Topics: map[string]eventcatalog.TopicConfig{},
			Events: map[string]eventcatalog.EventConfig{},
		}),
		Mode: PublishModeNop,
	})
	evt := event.New("unknown.event", "Unknown", "1", map[string]string{})

	if err := publisher.Publish(context.Background(), evt); err == nil {
		t.Fatalf("Publish should reject unknown event")
	}
}

func loadEventCatalog(t *testing.T) *eventcatalog.Catalog {
	t.Helper()
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	return eventcatalog.NewCatalog(cfg)
}

func assertPublishOutcome(t *testing.T, observer *publishObserver, outcome eventobservability.PublishOutcome) {
	t.Helper()
	if len(observer.events) != 1 {
		t.Fatalf("observed publish events = %#v, want one", observer.events)
	}
	if observer.events[0].Outcome != outcome {
		t.Fatalf("outcome = %q, want %q", observer.events[0].Outcome, outcome)
	}
	if observer.events[0].EventType == "" && outcome != eventobservability.PublishOutcomeUnknownEvent {
		t.Fatalf("event type was not captured")
	}
}
