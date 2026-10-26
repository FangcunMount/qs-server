package messaging

import (
	"context"
	"io"
	"log/slog"

	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
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

type consumeObserver struct {
	events    []eventobservability.ConsumeEvent
	durations []eventobservability.ConsumeDurationEvent
}

func (o *consumeObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (o *consumeObserver) ObserveOutbox(context.Context, eventobservability.OutboxEvent)   {}

func (o *consumeObserver) ObserveConsume(_ context.Context, evt eventobservability.ConsumeEvent) {
	o.events = append(o.events, evt)
}

func (o *consumeObserver) ObserveConsumeDuration(_ context.Context, evt eventobservability.ConsumeDurationEvent) {
	o.durations = append(o.durations, evt)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
