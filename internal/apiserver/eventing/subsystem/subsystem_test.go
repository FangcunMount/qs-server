package subsystem

import (
	"context"
	"errors"
	"sync"
	"testing"

	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
)

const hotRankConsumerID = "modelcatalog.hot_rank_projection"

type fakePublisher struct{}

func (fakePublisher) PublishWire(context.Context, string, []byte) error { return nil }

type fakeSubscriber struct {
	topic    string
	channel  string
	handler  rmtransport.Handler
	stops    int
	closes   int
	closeErr error
}

func (s *fakeSubscriber) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	s.topic, s.channel, s.handler = topic, channel, handler
	return nil
}
func (s *fakeSubscriber) Stop()        { s.stops++ }
func (s *fakeSubscriber) Close() error { s.closes++; return s.closeErr }

func loadCatalog(t *testing.T) *eventcatalog.Catalog {
	t.Helper()
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load event catalog: %v", err)
	}
	return eventcatalog.NewCatalog(cfg)
}

func TestSubsystemRequiresEnabledConsumerBindingBeforeStart(t *testing.T) {
	s, err := New(Options{Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, WirePublisher: fakePublisher{}})
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
		Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, WirePublisher: fakePublisher{},
		SDKSubscriberFactory: func() (SDKSubscriber, error) { return subscriber, nil },
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

	delivery := &projectionSDKDeliveryStub{message: rmtransport.Received{
		ID: "event-1", Metadata: map[string]string{"event_type": eventcatalog.AnswerSheetSubmitted},
		Payload: []byte(`{"event_type":"answersheet.submitted"}`),
	}}
	if err := subscriber.handler(t.Context(), delivery); err != nil {
		t.Fatalf("handled message: %v", err)
	}
	if delivery.acks != 1 {
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
	for _, profile := range status.Profiles {
		if profile.Name == eventcatalog.OutboxProfileMongoDomain && (profile.RelayEnabled || profile.ImmediateEnabled) {
			t.Fatalf("logging profile status = %#v, want relay and immediate disabled", profile)
		}
	}
}

func TestSubsystemCloseAggregatesSubscriberErrorsAndRunsOnce(t *testing.T) {
	errA := errors.New("subscriber a close failed")
	errB := errors.New("subscriber b close failed")
	subscriberA := &fakeSubscriber{closeErr: errA}
	subscriberB := &fakeSubscriber{closeErr: errB}
	s := &Subsystem{
		closeDone: make(chan struct{}),
		consumers: map[string]*consumerRuntime{
			"a": {spec: eventcatalog.ConsumerSpec{ID: "a"}, subscriber: subscriberA, healthy: true},
			"b": {spec: eventcatalog.ConsumerSpec{ID: "b"}, subscriber: subscriberB, healthy: true},
		},
	}

	const callers = 8
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs[index] = s.Close()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if !errors.Is(err, errA) || !errors.Is(err, errB) {
			t.Fatalf("Close()[%d] error = %v, want joined subscriber errors", i, err)
		}
		if err != errs[0] {
			t.Fatalf("Close()[%d] error instance differs from first result", i)
		}
	}
	if subscriberA.stops != 1 || subscriberA.closes != 1 || subscriberB.stops != 1 || subscriberB.closes != 1 {
		t.Fatalf("subscriber lifecycle A=(%d,%d) B=(%d,%d), want each once",
			subscriberA.stops, subscriberA.closes, subscriberB.stops, subscriberB.closes)
	}
	status := s.runtimeStatusSnapshot()
	if status.Consumers["a"].Healthy || status.Consumers["b"].Healthy {
		t.Fatalf("consumer health after Close = %#v", status.Consumers)
	}
}
