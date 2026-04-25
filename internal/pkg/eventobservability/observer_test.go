package eventobservability

import (
	"context"
	"testing"
)

var _ Observer = NopObserver{}
var _ Observer = (*PrometheusObserver)(nil)

func TestNopObserverIsNilSafe(t *testing.T) {
	observer := NormalizeObserver(nil)
	observer.ObservePublish(context.Background(), PublishEvent{Outcome: PublishOutcomeMQPublished})
	observer.ObserveOutbox(context.Background(), OutboxEvent{Outcome: OutboxOutcomePublished})
	observer.ObserveConsume(context.Background(), ConsumeEvent{Outcome: ConsumeOutcomeAcked})
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
