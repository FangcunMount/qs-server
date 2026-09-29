package subsystem

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
)

type projectionSDKSubscriberStub struct {
	topic, channel string
	handler        rmtransport.Handler
	stops, closes  int
}

func (s *projectionSDKSubscriberStub) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	s.topic, s.channel, s.handler = topic, channel, handler
	return nil
}
func (s *projectionSDKSubscriberStub) Stop()        { s.stops++ }
func (s *projectionSDKSubscriberStub) Close() error { s.closes++; return nil }

type projectionSDKDeliveryStub struct {
	message rmtransport.Received
	acks    int
	ackErr  error
}

func (d *projectionSDKDeliveryStub) Message() rmtransport.Received { return d.message }
func (d *projectionSDKDeliveryStub) Ack() error {
	d.acks++
	return d.ackErr
}
func (*projectionSDKDeliveryStub) Nack(error) error { return nil }
func (d *projectionSDKDeliveryStub) Settled() bool  { return d.acks > 0 }

type projectionSDKObserverStub struct {
	eventobservability.NopObserver
	events []eventobservability.ConsumeEvent
}

func (o *projectionSDKObserverStub) ObserveConsume(_ context.Context, event eventobservability.ConsumeEvent) {
	o.events = append(o.events, event)
}

func TestSDKProjectionSubscriberSettlesOnlyAfterHandlerSuccess(t *testing.T) {
	subscriber := &projectionSDKSubscriberStub{}
	observer := &projectionSDKObserverStub{}
	s, err := New(Options{
		Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, WirePublisher: fakePublisher{},
		SDKSubscriberFactory: func() (SDKSubscriber, error) { return subscriber, nil }, Observer: observer,
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	var handlerErr error
	if err := s.RegisterConsumer(hotRankConsumerID, func(_ context.Context, eventType string, payload []byte) error {
		calls++
		if eventType != eventcatalog.AnswerSheetSubmitted || len(payload) == 0 {
			t.Fatalf("projection input = %q / %q", eventType, payload)
		}
		return handlerErr
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	if subscriber.handler == nil || subscriber.channel != "qs-apiserver-modelcatalog-hot-rank-v1" {
		t.Fatalf("SDK subscription = %q/%q", subscriber.topic, subscriber.channel)
	}
	for _, item := range []struct {
		name, eventType, payload string
		wantCalls, wantAcks      int
		wantError                bool
		outcome                  eventobservability.ConsumeOutcome
	}{
		{"invalid envelope", "", "not-json", 0, 0, true, eventobservability.ConsumeOutcomeDecodeFailed},
		{"other event on topic", "other.event", "other", 0, 1, false, eventobservability.ConsumeOutcomeUnknownAcked},
		{"handler failure", eventcatalog.AnswerSheetSubmitted, "business", 1, 0, true, eventobservability.ConsumeOutcomeDispatchFailed},
		{"handler success", eventcatalog.AnswerSheetSubmitted, "business", 2, 1, false, eventobservability.ConsumeOutcomeAcked},
	} {
		t.Run(item.name, func(t *testing.T) {
			if item.name == "handler failure" {
				handlerErr = errors.New("projection store unavailable")
			} else {
				handlerErr = nil
			}
			metadata := map[string]string{}
			if item.eventType != "" {
				metadata["event_type"] = item.eventType
			}
			delivery := &projectionSDKDeliveryStub{message: rmtransport.Received{
				ID: item.name, Metadata: metadata, Payload: []byte(item.payload), Attempts: 2,
			}}
			err := subscriber.handler(t.Context(), delivery)
			if (err != nil) != item.wantError || delivery.acks != item.wantAcks || calls != item.wantCalls {
				t.Fatalf("result err=%v acks=%d calls=%d", err, delivery.acks, calls)
			}
			if got := observer.events[len(observer.events)-1]; got.Outcome != item.outcome || got.Attempts != 2 || got.Service != hotRankConsumerID {
				t.Fatalf("observation = %+v", got)
			}
		})
	}
	want := errors.New("broker ACK unavailable")
	delivery := &projectionSDKDeliveryStub{message: rmtransport.Received{
		ID: "ack-error", Metadata: map[string]string{"event_type": eventcatalog.AnswerSheetSubmitted}, Payload: []byte("business"),
	}, ackErr: want}
	if err := subscriber.handler(t.Context(), delivery); !errors.Is(err, want) || delivery.acks != 1 {
		t.Fatalf("ACK failure = %v, attempts = %d", err, delivery.acks)
	}
	if observer.events[len(observer.events)-1].Outcome != eventobservability.ConsumeOutcomeAckFailed {
		t.Fatalf("ACK observation = %+v", observer.events[len(observer.events)-1])
	}
	if err := s.Close(); err != nil || subscriber.stops != 1 || subscriber.closes != 1 {
		t.Fatalf("SDK subscriber lifecycle err=%v stops=%d closes=%d", err, subscriber.stops, subscriber.closes)
	}
}
