package messaging

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
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

func TestEnsureChannelsPreparesWorkerBeforeSubscription(t *testing.T) {
	requests := make([]string, 0, 2)
	nsqd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path+"?"+r.URL.RawQuery)
	}))
	defer nsqd.Close()
	source := &fakeSubscriptionRuntime{subs: []eventcatalog.TopicSubscription{{TopicName: "qs.evaluation.lifecycle"}}}
	cfg := &workerconfig.MessagingConfig{NSQDHTTPEndpoints: []string{nsqd.URL}}
	if err := EnsureChannels(context.Background(), cfg, "qs-worker", source); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/topic/create?topic=qs.evaluation.lifecycle",
		"/channel/create?channel=qs-worker&topic=qs.evaluation.lifecycle",
	}
	if len(requests) != len(want) || requests[0] != want[0] || requests[1] != want[1] {
		t.Fatalf("NSQ preparation sequence = %q, want %q", requests, want)
	}
}

func TestEnsureChannelsStopsOnUnavailableNSQD(t *testing.T) {
	nsqd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer nsqd.Close()
	cfg := &workerconfig.MessagingConfig{NSQDHTTPEndpoints: []string{nsqd.URL}}
	source := &fakeSubscriptionRuntime{subs: []eventcatalog.TopicSubscription{{TopicName: "qs.evaluation.lifecycle"}}}
	if err := EnsureChannels(context.Background(), cfg, "qs-worker", source); err == nil {
		t.Fatal("unavailable nsqd was treated as successful channel preparation")
	}
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
