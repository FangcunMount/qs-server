package subsystem

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
)

const hotRankConsumerID = "modelcatalog.hot_rank_projection"

type fakePublisher struct{}

func (fakePublisher) Publish(context.Context, string, []byte) error                    { return nil }
func (fakePublisher) PublishMessage(context.Context, string, *messaging.Message) error { return nil }
func (fakePublisher) Close() error                                                     { return nil }

type fakeSubscriber struct {
	topic    string
	channel  string
	handler  messaging.Handler
	stops    int
	closes   int
	closeErr error
}

func (s *fakeSubscriber) Subscribe(topic, channel string, handler messaging.Handler) error {
	s.topic, s.channel, s.handler = topic, channel, handler
	return nil
}
func (s *fakeSubscriber) SubscribeWithMiddleware(topic, channel string, handler messaging.Handler, _ ...messaging.Middleware) error {
	return s.Subscribe(topic, channel, handler)
}
func (s *fakeSubscriber) Stop()        { s.stops++ }
func (s *fakeSubscriber) Close() error { s.closes++; return s.closeErr }

type lifecycleRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *lifecycleRecorder) add(call string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *lifecycleRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

type fakeReconciler struct {
	name     string
	recorder *lifecycleRecorder
}

func (r *fakeReconciler) Start(context.Context) { r.recorder.add("reconciler.start." + r.name) }
func (r *fakeReconciler) Close()                { r.recorder.add("reconciler.close." + r.name) }

type fakeImmediate struct {
	name     string
	recorder *lifecycleRecorder
}

func (i *fakeImmediate) Close() { i.recorder.add("immediate.close." + i.name) }

type fakeRelay struct {
	name     string
	recorder *lifecycleRecorder
	started  chan struct{}
	once     sync.Once
}

func (r *fakeRelay) DispatchDue(ctx context.Context) error {
	r.once.Do(func() {
		r.recorder.add("relay.start." + r.name)
		close(r.started)
	})
	<-ctx.Done()
	r.recorder.add("relay.stop." + r.name)
	return ctx.Err()
}

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

func TestProjectionDecodeFailureNacksMessage(t *testing.T) {
	subscriber := &fakeSubscriber{}
	s, err := New(Options{
		Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, MQPublisher: fakePublisher{},
		SubscriberFactory: func() (messaging.Subscriber, error) { return subscriber, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterConsumer(hotRankConsumerID, func(context.Context, string, []byte) error {
		t.Fatal("projection handler must not run for an invalid envelope")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	var acked, nacked bool
	msg := messaging.NewMessage("invalid-event", []byte(`{"not":"an-envelope"}`))
	msg.SetAckFunc(func() error { acked = true; return nil })
	msg.SetNackFunc(func() error { nacked = true; return nil })
	if err := subscriber.handler(t.Context(), msg); err == nil {
		t.Fatal("invalid envelope error = nil")
	}
	if acked || !nacked {
		t.Fatalf("settlement acked=%v nacked=%v, want false/true", acked, nacked)
	}
}

func TestProjectionDecodeFailureReturnsNackError(t *testing.T) {
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
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	wantErr := errors.New("nack unavailable")
	msg := messaging.NewMessage("invalid-event", []byte(`{}`))
	msg.SetNackFunc(func() error { return wantErr })
	if err := subscriber.handler(t.Context(), msg); !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v, want %v", err, wantErr)
	}
}

func TestLoggingModeReportsProjectionConsumerDisabled(t *testing.T) {
	recorder := &lifecycleRecorder{}
	relay := &fakeRelay{name: "mongo", recorder: recorder, started: make(chan struct{})}
	s, err := New(Options{Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeLogging})
	if err != nil {
		t.Fatal(err)
	}
	s.profiles[eventcatalog.OutboxProfileMongoDomain] = &profileRuntime{
		name:       "mongo",
		relay:      relay,
		reconciler: &fakeReconciler{name: "mongo", recorder: recorder},
		interval:   time.Hour,
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatalf("logging Start(): %v", err)
	}
	defer func() { _ = s.Close() }()
	select {
	case <-relay.started:
		t.Fatal("logging mode started durable relay")
	default:
	}

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

func TestSubsystemStartsLifecycleInPhasesAndClosesProfilesInReverseOrder(t *testing.T) {
	recorder := &lifecycleRecorder{}
	mongoRelay := &fakeRelay{name: "mongo", recorder: recorder, started: make(chan struct{})}
	assessmentRelay := &fakeRelay{name: "assessment", recorder: recorder, started: make(chan struct{})}
	s, err := New(Options{
		Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, MQPublisher: fakePublisher{},
		Consumers: map[string]ConsumerOptions{hotRankConsumerID: {Enabled: false}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.profiles[eventcatalog.OutboxProfileMongoDomain] = &profileRuntime{
		name: "mongo", relay: mongoRelay,
		reconciler: &fakeReconciler{name: "mongo", recorder: recorder},
		immediate:  &fakeImmediate{name: "mongo", recorder: recorder},
		interval:   time.Hour,
	}
	s.profiles[eventcatalog.OutboxProfileAssessmentMySQL] = &profileRuntime{
		name: "assessment", relay: assessmentRelay,
		reconciler: &fakeReconciler{name: "assessment", recorder: recorder},
		immediate:  &fakeImmediate{name: "assessment", recorder: recorder},
		interval:   time.Hour,
	}

	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-mongoRelay.started
	<-assessmentRelay.started
	started := recorder.snapshot()
	if len(started) < 4 || !reflect.DeepEqual(started[:2], []string{
		"reconciler.start.mongo",
		"reconciler.start.assessment",
	}) {
		t.Fatalf("start lifecycle = %v, want all reconcilers in profile order before relays", started)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	runtimeStatus := s.runtimeStatusSnapshot()
	for profile, status := range runtimeStatus.Profiles {
		if status.Running {
			t.Fatalf("profile %s still running after Close: %#v", profile, status)
		}
	}
	closed := recorder.snapshot()
	wantSuffix := []string{
		"reconciler.close.assessment",
		"immediate.close.assessment",
		"reconciler.close.mongo",
		"immediate.close.mongo",
	}
	if len(closed) < len(wantSuffix) || !reflect.DeepEqual(closed[len(closed)-len(wantSuffix):], wantSuffix) {
		t.Fatalf("close lifecycle = %v, want suffix %v", closed, wantSuffix)
	}
}

func TestSubsystemCloseAggregatesSubscriberErrorsAndRunsOnce(t *testing.T) {
	errA := errors.New("subscriber a close failed")
	errB := errors.New("subscriber b close failed")
	subscriberA := &fakeSubscriber{closeErr: errA}
	subscriberB := &fakeSubscriber{closeErr: errB}
	s := &Subsystem{
		profiles:  map[eventcatalog.OutboxProfile]*profileRuntime{},
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
