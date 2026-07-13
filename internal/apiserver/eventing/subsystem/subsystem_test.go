package subsystem

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventruntime"
)

const hotRankConsumerID = "modelcatalog.hot_rank_projection"

type fakePublisher struct{}

func (fakePublisher) Publish(context.Context, string, []byte) error                    { return nil }
func (fakePublisher) PublishMessage(context.Context, string, *messaging.Message) error { return nil }
func (fakePublisher) Close() error                                                     { return nil }

type fakeSubscriber struct {
	topic   string
	channel string
	handler messaging.Handler
	stops   int
	closes  int
}

func (s *fakeSubscriber) Subscribe(topic, channel string, handler messaging.Handler) error {
	s.topic, s.channel, s.handler = topic, channel, handler
	return nil
}
func (s *fakeSubscriber) SubscribeWithMiddleware(topic, channel string, handler messaging.Handler, _ ...messaging.Middleware) error {
	return s.Subscribe(topic, channel, handler)
}
func (s *fakeSubscriber) Stop()        { s.stops++ }
func (s *fakeSubscriber) Close() error { s.closes++; return nil }

func loadCatalog(t *testing.T) *eventcatalog.Catalog {
	t.Helper()
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load event catalog: %v", err)
	}
	return eventcatalog.NewCatalog(cfg)
}

func TestSubsystemRequiresEnabledConsumerBindingBeforeStart(t *testing.T) {
	s, err := New(Options{Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, MQPublisher: fakePublisher{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err == nil {
		t.Fatal("Start() error = nil, want missing binding error")
	}
}

func TestSubsystemStartCloseAreIdempotentAndSettleProjectionMessages(t *testing.T) {
	subscriber := &fakeSubscriber{}
	s, err := New(Options{
		Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, MQPublisher: fakePublisher{},
		SubscriberFactory: func() (messaging.Subscriber, error) { return subscriber, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterConsumer(hotRankConsumerID, func(context.Context, string, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterConsumer(hotRankConsumerID, func(context.Context, string, []byte) error { return nil }); err == nil {
		t.Fatal("duplicate RegisterConsumer() error = nil")
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatalf("second Start(): %v", err)
	}
	if subscriber.handler == nil || subscriber.channel != "qs-apiserver-modelcatalog-hot-rank-v1" {
		t.Fatalf("subscription = topic %q channel %q handler %v", subscriber.topic, subscriber.channel, subscriber.handler != nil)
	}

	var acked bool
	msg := messaging.NewMessage("event-1", []byte(`{"event_type":"answersheet.submitted"}`))
	msg.Metadata["event_type"] = eventcatalog.AnswerSheetSubmitted
	msg.SetAckFunc(func() error { acked = true; return nil })
	if err := subscriber.handler(t.Context(), msg); err != nil {
		t.Fatalf("handled message: %v", err)
	}
	if !acked {
		t.Fatal("handled message was not ACKed")
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close(): %v", err)
	}
	if subscriber.stops != 1 || subscriber.closes != 1 {
		t.Fatalf("subscriber lifecycle stops=%d closes=%d", subscriber.stops, subscriber.closes)
	}
}

func TestProjectionHandlerFailureNacksOnlyItsMessage(t *testing.T) {
	subscriber := &fakeSubscriber{}
	s, err := New(Options{
		Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, MQPublisher: fakePublisher{},
		SubscriberFactory: func() (messaging.Subscriber, error) { return subscriber, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("redis unavailable")
	if err := s.RegisterConsumer(hotRankConsumerID, func(context.Context, string, []byte) error { return wantErr }); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	var nacked bool
	msg := messaging.NewMessage("event-2", nil)
	msg.Metadata["event_type"] = eventcatalog.AnswerSheetSubmitted
	msg.SetNackFunc(func() error { nacked = true; return nil })
	if err := subscriber.handler(t.Context(), msg); !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v, want %v", err, wantErr)
	}
	if !nacked {
		t.Fatal("handler failure was not NACKed")
	}
}

func TestLoggingModeReportsProjectionConsumerDisabled(t *testing.T) {
	s, err := New(Options{Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeLogging})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatalf("logging Start(): %v", err)
	}
	defer func() { _ = s.Close() }()

	status, err := s.StatusService().GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Consumers) != 1 || status.Consumers[0].Enabled {
		t.Fatalf("logging consumer status = %#v, want disabled", status.Consumers)
	}
}
