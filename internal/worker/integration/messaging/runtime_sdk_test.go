package messaging

import (
	"context"
	"errors"
	"testing"

	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
)

type sdkDeliveryStub struct {
	message   rmtransport.Received
	ackCount  int
	nackCount int
	ackErr    error
}

func (d *sdkDeliveryStub) Message() rmtransport.Received { return d.message }
func (d *sdkDeliveryStub) Ack() error {
	d.ackCount++
	return d.ackErr
}
func (d *sdkDeliveryStub) Nack(error) error {
	d.nackCount++
	return nil
}
func (d *sdkDeliveryStub) Settled() bool { return d.ackCount+d.nackCount > 0 }

type sdkHoldStub struct {
	calls int
	got   rmtransport.Received
	err   error
}

type capturedSDKSubscriber struct {
	topic, channel string
	handler        rmtransport.Handler
}

func (s *capturedSDKSubscriber) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	s.topic, s.channel, s.handler = topic, channel, handler
	return nil
}

func TestSDKWorkerSubscriptionRequiresAuditAndUsesCatalog(t *testing.T) {
	runtime := &fakeSubscriptionRuntime{subs: []eventcatalog.TopicSubscription{{TopicName: "sample.topic", EventTypes: []string{"sample.created"}}}}
	subscriber := &capturedSDKSubscriber{}
	options := SubscribeSDKHandlersOptions{ServiceName: "worker-channel", Runtime: runtime, Subscriber: subscriber}
	if err := SubscribeSDKHandlersWithOptions(options); err == nil {
		t.Fatal("Worker accepted a subscription without durable unknown-event audit")
	}
	options.UnknownRecorder = func(context.Context, rmtransport.Received, string) error { return nil }
	if err := SubscribeSDKHandlersWithOptions(options); err != nil {
		t.Fatal(err)
	}
	if subscriber.topic != "sample.topic" || subscriber.channel != "worker-channel" || subscriber.handler == nil {
		t.Fatalf("subscription: topic=%q channel=%q handler=%v", subscriber.topic, subscriber.channel, subscriber.handler != nil)
	}
}

func TestSDKWorkerDeliveryFallsBackToCanonicalEnvelope(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	delivery := &sdkDeliveryStub{message: rmtransport.Received{
		ID: "app-envelope", Payload: []byte(`{"id":"app-envelope","eventType":"payload.event"}`),
	}}
	if err := createSDKDispatchHandler(testLogger(), dispatcher, "topic", "worker", &consumeObserver{}, nil, nil)(t.Context(), delivery); err != nil {
		t.Fatal(err)
	}
	if dispatcher.eventType != "payload.event" || delivery.ackCount != 1 {
		t.Fatalf("envelope fallback: event=%q ack=%d", dispatcher.eventType, delivery.ackCount)
	}
}

func (h *sdkHoldStub) HoldDelivery(_ context.Context, msg rmtransport.Received, _ string, _ error) error {
	h.calls++
	h.got = msg
	return h.err
}

func TestSDKWorkerDeliveryAcksOnlyAfterBusinessAndDurableEvidence(t *testing.T) {
	t.Run("handled", func(t *testing.T) {
		dispatcher := &fakeDispatcher{}
		observer := &consumeObserver{}
		delivery := &sdkDeliveryStub{message: rmtransport.Received{
			ID: "app-1", TransportID: "physical-1", Topic: "topic", Channel: "worker",
			Metadata: map[string]string{"event_type": "evaluation.requested"}, Payload: []byte("not-json"), Attempts: 2,
		}}
		handler := createSDKDispatchHandler(testLogger(), dispatcher, "topic", "worker", observer, nil, nil)
		if err := handler(t.Context(), delivery); err != nil {
			t.Fatal(err)
		}
		if dispatcher.eventType != "evaluation.requested" || delivery.ackCount != 1 || delivery.nackCount != 0 || len(observer.events) != 1 || observer.events[0].Outcome != eventobservability.ConsumeOutcomeAcked || observer.events[0].Attempts != 2 {
			t.Fatalf("handled outcome: dispatcher=%#v delivery=%#v observations=%#v", dispatcher, delivery, observer.events)
		}
	})
	t.Run("ack failure", func(t *testing.T) {
		wantErr := errors.New("ack failed")
		delivery := &sdkDeliveryStub{
			message: rmtransport.Received{ID: "app-ack", Metadata: map[string]string{"event_type": "evaluation.requested"}},
			ackErr:  wantErr,
		}
		observer := &consumeObserver{}
		handler := createSDKDispatchHandler(testLogger(), &fakeDispatcher{}, "topic", "worker", observer, nil, nil)
		if err := handler(t.Context(), delivery); !errors.Is(err, wantErr) || delivery.ackCount != 1 || len(observer.events) != 1 || observer.events[0].Outcome != eventobservability.ConsumeOutcomeAckFailed {
			t.Fatalf("ack failure: err=%v delivery=%#v observations=%#v", err, delivery, observer.events)
		}
	})
	t.Run("unknown audit failed", func(t *testing.T) {
		dispatcher := &fakeDispatcher{outcome: eventruntime.DispatchUnknown}
		delivery := &sdkDeliveryStub{message: rmtransport.Received{ID: "app-2", Payload: []byte(`{"id":"app-2","eventType":"future.event"}`)}}
		wantErr := errors.New("audit unavailable")
		handler := createSDKDispatchHandler(testLogger(), dispatcher, "topic", "worker", &consumeObserver{}, nil,
			func(context.Context, rmtransport.Received, string) error { return wantErr })
		if err := handler(t.Context(), delivery); !errors.Is(err, wantErr) {
			t.Fatalf("unknown audit error = %v", err)
		}
		if delivery.Settled() {
			t.Fatal("unknown event was acknowledged before durable audit")
		}
	})
	t.Run("unknown audit committed", func(t *testing.T) {
		dispatcher := &fakeDispatcher{outcome: eventruntime.DispatchUnknown}
		delivery := &sdkDeliveryStub{message: rmtransport.Received{ID: "app-3", Metadata: map[string]string{"event_type": "future.event"}}}
		called := false
		handler := createSDKDispatchHandler(testLogger(), dispatcher, "topic", "worker", &consumeObserver{}, nil,
			func(_ context.Context, msg rmtransport.Received, eventType string) error {
				called = msg.ID == "app-3" && eventType == "future.event"
				return nil
			})
		if err := handler(t.Context(), delivery); err != nil || !called || delivery.ackCount != 1 {
			t.Fatalf("unknown success: err=%v called=%v delivery=%#v", err, called, delivery)
		}
	})
	t.Run("paused retry hold failed", func(t *testing.T) {
		dispatcher := &fakeDispatcher{err: eventruntime.ErrAutomaticRetryPaused}
		hold := &sdkHoldStub{err: errors.New("mysql unavailable")}
		delivery := &sdkDeliveryStub{message: rmtransport.Received{ID: "app-4", Topic: "topic", Channel: "worker", Metadata: map[string]string{"event_type": "evaluation.retry.requested"}}}
		handler := createSDKDispatchHandler(testLogger(), dispatcher, "topic", "worker", &consumeObserver{}, hold, nil)
		if err := handler(t.Context(), delivery); !errors.Is(err, hold.err) || hold.calls != 1 || delivery.Settled() {
			t.Fatalf("failed hold: err=%v hold=%#v delivery=%#v", err, hold, delivery)
		}
	})
	t.Run("paused retry held", func(t *testing.T) {
		dispatcher := &fakeDispatcher{err: eventruntime.ErrAutomaticRetryPaused}
		hold := &sdkHoldStub{}
		delivery := &sdkDeliveryStub{message: rmtransport.Received{ID: "app-5", TransportID: "physical-5", Topic: "topic", Channel: "worker", Metadata: map[string]string{"event_type": "evaluation.retry.requested"}}}
		handler := createSDKDispatchHandler(testLogger(), dispatcher, "topic", "worker", &consumeObserver{}, hold, nil)
		if err := handler(t.Context(), delivery); err != nil || hold.calls != 1 || hold.got.TransportID != "physical-5" || delivery.ackCount != 1 {
			t.Fatalf("held retry: err=%v hold=%#v delivery=%#v", err, hold, delivery)
		}
	})
	t.Run("malformed envelope", func(t *testing.T) {
		delivery := &sdkDeliveryStub{message: rmtransport.Received{ID: "app-6", Payload: []byte("not-json")}}
		handler := createSDKDispatchHandler(testLogger(), &fakeDispatcher{}, "topic", "worker", &consumeObserver{}, nil, nil)
		if err := handler(t.Context(), delivery); err == nil || delivery.Settled() {
			t.Fatalf("malformed delivery: err=%v settled=%v", err, delivery.Settled())
		}
	})
}
