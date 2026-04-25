package eventobservability

import (
	"context"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

var _ Observer = NopObserver{}
var _ Observer = (*PrometheusObserver)(nil)
var _ ConsumeDurationObserver = NopObserver{}
var _ ConsumeDurationObserver = (*PrometheusObserver)(nil)
var _ OutboxStatusObserver = NopObserver{}
var _ OutboxStatusObserver = (*PrometheusObserver)(nil)
var _ OutboxStatusScrapeObserver = NopObserver{}
var _ OutboxStatusScrapeObserver = (*PrometheusObserver)(nil)

func TestNopObserverIsNilSafe(t *testing.T) {
	observer := NormalizeObserver(nil)
	observer.ObservePublish(context.Background(), PublishEvent{Outcome: PublishOutcomeMQPublished})
	observer.ObserveOutbox(context.Background(), OutboxEvent{Outcome: OutboxOutcomePublished})
	observer.ObserveConsume(context.Background(), ConsumeEvent{Outcome: ConsumeOutcomeAcked})
	ObserveConsumeDuration(context.Background(), observer, ConsumeDurationEvent{Outcome: ConsumeOutcomeAcked})
	ObserveOutboxStatus(context.Background(), observer, OutboxStatusEvent{Store: "store", Status: "pending"})
	ObserveOutboxStatusScrape(context.Background(), observer, OutboxStatusScrapeEvent{Store: "store", Outcome: OutboxStatusScrapeOutcomeSuccess})
}

func TestDefaultObserverIsPrometheusObserver(t *testing.T) {
	if _, ok := DefaultObserver().(PrometheusObserver); !ok {
		t.Fatalf("DefaultObserver() = %T, want PrometheusObserver", DefaultObserver())
	}
}

func TestOutcomeStringValuesAreStable(t *testing.T) {
	cases := map[string]string{
		PublishOutcomeMQPublished.String():        "mq_published",
		PublishOutcomeFallbackLogged.String():     "fallback_logged",
		PublishOutcomeLogged.String():             "logged",
		PublishOutcomeNop.String():                "nop",
		PublishOutcomeUnknownEvent.String():       "unknown_event",
		PublishOutcomeEncodeFailed.String():       "encode_failed",
		PublishOutcomeMQFailed.String():           "mq_failed",
		OutboxOutcomeClaimFailed.String():         "claim_failed",
		OutboxOutcomePublished.String():           "published",
		OutboxOutcomePublishFailed.String():       "publish_failed",
		OutboxOutcomeMarkFailedFailed.String():    "mark_failed_failed",
		OutboxOutcomeMarkPublishedFailed.String(): "mark_published_failed",
		OutboxStatusScrapeOutcomeSuccess.String(): "success",
		OutboxStatusScrapeOutcomeFailure.String(): "failure",
		ConsumeOutcomePoisonAcked.String():        "poison_acked",
		ConsumeOutcomePoisonAckFailed.String():    "poison_ack_failed",
		ConsumeOutcomeAcked.String():              "acked",
		ConsumeOutcomeAckFailed.String():          "ack_failed",
		ConsumeOutcomeNacked.String():             "nacked",
		ConsumeOutcomeNackFailed.String():         "nack_failed",
	}
	for got, want := range cases {
		if got == "" {
			t.Fatalf("outcome string is empty")
		}
		if got != want {
			t.Fatalf("outcome string = %q, want %q", got, want)
		}
	}
}

func TestEventMetricsUseBoundedLabels(t *testing.T) {
	descriptions := []string{
		describeCollector(eventConsumeDuration),
		describeCollector(eventOutboxBacklog),
		describeCollector(eventOutboxOldestAge),
		describeCollector(eventOutboxStatusScrapeTotal),
	}
	for _, desc := range descriptions {
		for _, forbidden := range []string{"event_id", "aggregate_id", "error", "last_error"} {
			if strings.Contains(desc, forbidden) {
				t.Fatalf("metric description %q contains forbidden high-cardinality label %q", desc, forbidden)
			}
		}
	}
}

func describeCollector(collector interface {
	Describe(chan<- *prometheus.Desc)
}) string {
	ch := make(chan *prometheus.Desc, 4)
	collector.Describe(ch)
	close(ch)
	var builder strings.Builder
	for desc := range ch {
		builder.WriteString(desc.String())
	}
	return builder.String()
}
