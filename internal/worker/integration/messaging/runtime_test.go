package messaging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
)

type fakeDispatcher struct {
	eventType string
	payload   []byte
	err       error
	outcome   eventruntime.DispatchOutcome
	calls     int
}

func (d *fakeDispatcher) DispatchEvent(_ context.Context, eventType string, payload []byte) (eventruntime.DispatchResult, error) {
	d.calls++
	d.eventType = eventType
	d.payload = payload
	outcome := d.outcome
	if outcome == "" {
		outcome = eventruntime.DispatchHandled
	}
	return eventruntime.DispatchResult{Outcome: outcome}, d.err
}

type fakeSubscriptionRuntime struct {
	fakeDispatcher
	subs []eventcatalog.TopicSubscription
}

func (r *fakeSubscriptionRuntime) GetTopicSubscriptions() []eventcatalog.TopicSubscription {
	return r.subs
}

type fakeSubscriber struct {
	topic   string
	channel string
	handler basemessaging.Handler
}

func (s *fakeSubscriber) Subscribe(topic, channel string, handler basemessaging.Handler) error {
	s.topic = topic
	s.channel = channel
	s.handler = handler
	return nil
}

func (s *fakeSubscriber) SubscribeWithMiddleware(topic, channel string, handler basemessaging.Handler, middlewares ...basemessaging.Middleware) error {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return s.Subscribe(topic, channel, handler)
}

func (*fakeSubscriber) Stop() {}

func (*fakeSubscriber) Close() error { return nil }

type consumeObserver struct {
	events    []eventobservability.ConsumeEvent
	durations []eventobservability.ConsumeDurationEvent
}

type fakeHoldRecorder struct {
	calls int
	err   error
}

func (r *fakeHoldRecorder) Hold(context.Context, *basemessaging.Message, string, error) error {
	r.calls++
	return r.err
}

func (o *consumeObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (o *consumeObserver) ObserveOutbox(context.Context, eventobservability.OutboxEvent)   {}

func (o *consumeObserver) ObserveConsume(_ context.Context, evt eventobservability.ConsumeEvent) {
	o.events = append(o.events, evt)
}

func (o *consumeObserver) ObserveConsumeDuration(_ context.Context, evt eventobservability.ConsumeDurationEvent) {
	o.durations = append(o.durations, evt)
}

func TestDispatchHandlerUsesMetadataEventTypeFirst(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	msg := basemessaging.NewMessage("msg-1", []byte("not-json"))
	msg.Metadata["event_type"] = "metadata.event"
	ackCount := 0
	msg.SetAckFunc(func() error {
		ackCount++
		return nil
	})

	handler := createDispatchHandler(testLogger(), dispatcher, "topic")
	if err := handler(context.Background(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if dispatcher.eventType != "metadata.event" {
		t.Fatalf("eventType = %q, want metadata.event", dispatcher.eventType)
	}
	if ackCount != 1 {
		t.Fatalf("ackCount = %d, want 1", ackCount)
	}
}

func TestDispatchHandlerFallsBackToPayloadEnvelope(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	payload := []byte(`{"id":"evt-1","eventType":"payload.event","occurredAt":"2026-04-25T00:00:00Z","aggregateType":"Sample","aggregateID":"sample-1","data":{"ok":true}}`)
	msg := basemessaging.NewMessage("msg-1", payload)

	handler := createDispatchHandler(testLogger(), dispatcher, "topic")
	if err := handler(context.Background(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}

	if dispatcher.eventType != "payload.event" {
		t.Fatalf("eventType = %q, want payload.event", dispatcher.eventType)
	}
	if msg.Metadata["event_type"] != "payload.event" {
		t.Fatalf("metadata event_type = %q, want payload.event", msg.Metadata["event_type"])
	}
}

func TestMessageEventExtractorUsesMetadataBeforePayload(t *testing.T) {
	msg := basemessaging.NewMessage("msg-1", []byte("not-json"))
	msg.Metadata["event_type"] = "metadata.event"

	eventType, err := (MessageEventExtractor{}).Extract(msg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if eventType != "metadata.event" {
		t.Fatalf("eventType = %q, want metadata.event", eventType)
	}
}

func TestMessageSettlementPolicyAckSuccess(t *testing.T) {
	msg := basemessaging.NewMessage("msg-1", []byte("{}"))
	ackCount := 0
	msg.SetAckFunc(func() error {
		ackCount++
		return nil
	})

	policy := eventruntime.NewMessageSettlementPolicy(testLogger(), "worker", "topic", nil)
	outcome, err := policy.AckSuccess(msg)
	if err != nil {
		t.Fatalf("AckSuccess: %v", err)
	}
	if outcome != eventobservability.ConsumeOutcomeAcked {
		t.Fatalf("outcome = %q, want acked", outcome)
	}
	if ackCount != 1 {
		t.Fatalf("ackCount = %d, want 1", ackCount)
	}
}

func TestSubscribeHandlersUsesNarrowSubscriptionRuntime(t *testing.T) {
	runtime := &fakeSubscriptionRuntime{
		subs: []eventcatalog.TopicSubscription{
			{TopicName: "sample.topic", EventTypes: []string{"sample.created"}},
		},
	}
	subscriber := &fakeSubscriber{}

	if err := SubscribeHandlers("worker-channel", testLogger(), runtime, subscriber); err != nil {
		t.Fatalf("SubscribeHandlers: %v", err)
	}
	if subscriber.topic != "sample.topic" {
		t.Fatalf("topic = %q, want sample.topic", subscriber.topic)
	}
	if subscriber.channel != "worker-channel" {
		t.Fatalf("channel = %q, want worker-channel", subscriber.channel)
	}
	if subscriber.handler == nil {
		t.Fatalf("handler = nil")
	}
}

func TestDispatchHandlerNacksInvalidPayloadWithoutDispatch(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	msg := basemessaging.NewMessage("msg-1", []byte("not-json"))
	nackCount := 0
	msg.SetNackFunc(func() error {
		nackCount++
		return nil
	})

	handler := createDispatchHandler(testLogger(), dispatcher, "topic")
	if err := handler(context.Background(), msg); err == nil {
		t.Fatal("expected decode error")
	}

	if dispatcher.calls != 0 {
		t.Fatalf("dispatch calls = %d, want 0", dispatcher.calls)
	}
	if nackCount != 1 {
		t.Fatalf("nackCount = %d, want 1", nackCount)
	}
}

func TestDispatchHandlerObservesDecodeNacked(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{}
	msg := basemessaging.NewMessage("msg-1", []byte("not-json"))
	msg.SetNackFunc(func() error { return nil })

	handler := createDispatchHandlerWithObserver(testLogger(), dispatcher, "topic", "worker", observer)
	if err := handler(context.Background(), msg); err == nil {
		t.Fatal("expected decode error")
	}

	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeDecodeNacked)
	assertNoConsumeDuration(t, observer)
}

func TestDispatchHandlerObservesDecodeNackFailed(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{}
	msg := basemessaging.NewMessage("msg-1", []byte("not-json"))
	msg.SetNackFunc(func() error { return errors.New("nack failed") })

	handler := createDispatchHandlerWithObserver(testLogger(), dispatcher, "topic", "worker", observer)
	if err := handler(context.Background(), msg); err == nil {
		t.Fatal("expected decode/nack error")
	}

	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeDecodeNackFailed)
	assertNoConsumeDuration(t, observer)
}

func TestDispatchHandlerNacksOnDispatchError(t *testing.T) {
	wantErr := errors.New("dispatch failed")
	dispatcher := &fakeDispatcher{err: wantErr}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	nackCount := 0
	msg.SetNackFunc(func() error {
		nackCount++
		return nil
	})

	handler := createDispatchHandler(testLogger(), dispatcher, "topic")
	if err := handler(context.Background(), msg); !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v, want %v", err, wantErr)
	}

	if nackCount != 1 {
		t.Fatalf("nackCount = %d, want 1", nackCount)
	}
}

func TestDispatchHandlerObservesNacked(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{err: errors.New("dispatch failed")}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	msg.SetNackFunc(func() error { return nil })

	handler := createDispatchHandlerWithObserver(testLogger(), dispatcher, "topic", "worker", observer)
	if err := handler(context.Background(), msg); err == nil {
		t.Fatalf("handler should return dispatch error")
	}

	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeNacked)
	assertConsumeDuration(t, observer, eventobservability.ConsumeOutcomeNacked)
}

func TestDispatchHandlerAcksPausedEventOnlyAfterDurableHold(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{err: eventruntime.ErrAutomaticRetryPaused}
	recorder := &fakeHoldRecorder{}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	ackCount := 0
	msg.SetAckFunc(func() error { ackCount++; return nil })

	handler := createDispatchHandlerWithObserverAndHold(testLogger(), dispatcher, "topic", "worker", observer, recorder)
	if err := handler(t.Context(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if recorder.calls != 1 || ackCount != 1 {
		t.Fatalf("hold calls=%d ack calls=%d, want 1/1", recorder.calls, ackCount)
	}
	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeHeld)
	assertConsumeDuration(t, observer, eventobservability.ConsumeOutcomeHeld)
}

func TestDispatchHandlerNacksPausedEventWhenHoldFails(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{err: eventruntime.ErrAutomaticRetryPaused}
	recorder := &fakeHoldRecorder{err: errors.New("mysql unavailable")}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	nackCount := 0
	msg.SetNackFunc(func() error { nackCount++; return nil })

	handler := createDispatchHandlerWithObserverAndHold(testLogger(), dispatcher, "topic", "worker", observer, recorder)
	if err := handler(t.Context(), msg); err == nil {
		t.Fatal("expected hold error")
	}
	if recorder.calls != 1 || nackCount != 1 {
		t.Fatalf("hold calls=%d nack calls=%d, want 1/1", recorder.calls, nackCount)
	}
	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeHoldFailed)
	assertNoConsumeDuration(t, observer)
}

func TestDispatchHandlerObservesNackFailed(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{err: errors.New("dispatch failed")}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	msg.SetNackFunc(func() error { return errors.New("nack failed") })

	handler := createDispatchHandlerWithObserver(testLogger(), dispatcher, "topic", "worker", observer)
	if err := handler(context.Background(), msg); err == nil {
		t.Fatalf("handler should return dispatch error")
	}

	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeNackFailed)
	assertConsumeDuration(t, observer, eventobservability.ConsumeOutcomeNackFailed)
}

func TestDispatchHandlerObservesAcked(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	msg.SetAckFunc(func() error { return nil })

	handler := createDispatchHandlerWithObserver(testLogger(), dispatcher, "topic", "worker", observer)
	if err := handler(context.Background(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}

	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeAcked)
	assertConsumeDuration(t, observer, eventobservability.ConsumeOutcomeAcked)
}

func TestDispatchHandlerObservesUnknownAcked(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{outcome: eventruntime.DispatchUnknown}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	msg.SetAckFunc(func() error { return nil })

	handler := createDispatchHandlerWithObserver(testLogger(), dispatcher, "topic", "worker", observer)
	if err := handler(context.Background(), msg); err != nil {
		t.Fatalf("handler: %v", err)
	}

	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeUnknownAcked)
	assertConsumeDuration(t, observer, eventobservability.ConsumeOutcomeUnknownAcked)
}

func TestDispatchHandlerObservesAckFailed(t *testing.T) {
	observer := &consumeObserver{}
	dispatcher := &fakeDispatcher{}
	msg := basemessaging.NewMessage("msg-1", []byte(`{}`))
	msg.Metadata["event_type"] = "metadata.event"
	msg.SetAckFunc(func() error { return errors.New("ack failed") })

	handler := createDispatchHandlerWithObserver(testLogger(), dispatcher, "topic", "worker", observer)
	if err := handler(context.Background(), msg); err == nil {
		t.Fatalf("handler should return ack error")
	}

	assertConsumeOutcome(t, observer, eventobservability.ConsumeOutcomeAckFailed)
	assertConsumeDuration(t, observer, eventobservability.ConsumeOutcomeAckFailed)
}

func assertConsumeOutcome(t *testing.T, observer *consumeObserver, outcome eventobservability.ConsumeOutcome) {
	t.Helper()
	if len(observer.events) != 1 {
		t.Fatalf("observed consume events = %#v, want one", observer.events)
	}
	evt := observer.events[0]
	if evt.Outcome != outcome {
		t.Fatalf("outcome = %q, want %q", evt.Outcome, outcome)
	}
	if evt.Service != "worker" {
		t.Fatalf("service = %q, want worker", evt.Service)
	}
	if evt.Topic != "topic" {
		t.Fatalf("topic = %q, want topic", evt.Topic)
	}
}

func assertConsumeDuration(t *testing.T, observer *consumeObserver, outcome eventobservability.ConsumeOutcome) {
	t.Helper()
	if len(observer.durations) != 1 {
		t.Fatalf("observed consume durations = %#v, want one", observer.durations)
	}
	evt := observer.durations[0]
	if evt.Outcome != outcome {
		t.Fatalf("duration outcome = %q, want %q", evt.Outcome, outcome)
	}
	if evt.Service != "worker" {
		t.Fatalf("duration service = %q, want worker", evt.Service)
	}
	if evt.Topic != "topic" {
		t.Fatalf("duration topic = %q, want topic", evt.Topic)
	}
	if evt.EventType != "metadata.event" {
		t.Fatalf("duration event_type = %q, want metadata.event", evt.EventType)
	}
	if evt.Duration < 0 {
		t.Fatalf("duration = %v, want non-negative", evt.Duration)
	}
}

func assertNoConsumeDuration(t *testing.T, observer *consumeObserver) {
	t.Helper()
	if len(observer.durations) != 0 {
		t.Fatalf("observed consume durations = %#v, want none", observer.durations)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
