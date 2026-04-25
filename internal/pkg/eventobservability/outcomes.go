// Package eventobservability defines bounded outcome labels for the event system.
package eventobservability

import (
	"context"
	"time"
)

type PublishOutcome string

const (
	PublishOutcomeMQPublished    PublishOutcome = "mq_published"
	PublishOutcomeFallbackLogged PublishOutcome = "fallback_logged"
	PublishOutcomeLogged         PublishOutcome = "logged"
	PublishOutcomeNop            PublishOutcome = "nop"
	PublishOutcomeUnknownEvent   PublishOutcome = "unknown_event"
	PublishOutcomeEncodeFailed   PublishOutcome = "encode_failed"
	PublishOutcomeMQFailed       PublishOutcome = "mq_failed"
)

func (o PublishOutcome) String() string { return string(o) }

type OutboxOutcome string

const (
	OutboxOutcomeClaimFailed         OutboxOutcome = "claim_failed"
	OutboxOutcomePublished           OutboxOutcome = "published"
	OutboxOutcomePublishFailed       OutboxOutcome = "publish_failed"
	OutboxOutcomeMarkFailedFailed    OutboxOutcome = "mark_failed_failed"
	OutboxOutcomeMarkPublishedFailed OutboxOutcome = "mark_published_failed"
)

func (o OutboxOutcome) String() string { return string(o) }

type OutboxStatusScrapeOutcome string

const (
	OutboxStatusScrapeOutcomeSuccess OutboxStatusScrapeOutcome = "success"
	OutboxStatusScrapeOutcomeFailure OutboxStatusScrapeOutcome = "failure"
)

func (o OutboxStatusScrapeOutcome) String() string { return string(o) }

type ConsumeOutcome string

const (
	ConsumeOutcomePoisonAcked     ConsumeOutcome = "poison_acked"
	ConsumeOutcomePoisonAckFailed ConsumeOutcome = "poison_ack_failed"
	ConsumeOutcomeAcked           ConsumeOutcome = "acked"
	ConsumeOutcomeAckFailed       ConsumeOutcome = "ack_failed"
	ConsumeOutcomeNacked          ConsumeOutcome = "nacked"
	ConsumeOutcomeNackFailed      ConsumeOutcome = "nack_failed"
)

func (o ConsumeOutcome) String() string { return string(o) }

type PublishEvent struct {
	Source    string
	Mode      string
	Topic     string
	EventType string
	Outcome   PublishOutcome
}

type OutboxEvent struct {
	Relay     string
	Topic     string
	EventType string
	Outcome   OutboxOutcome
}

type ConsumeEvent struct {
	Service   string
	Topic     string
	EventType string
	Outcome   ConsumeOutcome
}

type ConsumeDurationEvent struct {
	Service   string
	Topic     string
	EventType string
	Outcome   ConsumeOutcome
	Duration  time.Duration
}

type OutboxStatusEvent struct {
	Store            string
	Status           string
	Count            int64
	OldestAgeSeconds float64
}

type OutboxStatusScrapeEvent struct {
	Store   string
	Outcome OutboxStatusScrapeOutcome
}

type Observer interface {
	ObservePublish(context.Context, PublishEvent)
	ObserveOutbox(context.Context, OutboxEvent)
	ObserveConsume(context.Context, ConsumeEvent)
}

type ConsumeDurationObserver interface {
	ObserveConsumeDuration(context.Context, ConsumeDurationEvent)
}

type OutboxStatusObserver interface {
	ObserveOutboxStatus(context.Context, OutboxStatusEvent)
}

type OutboxStatusScrapeObserver interface {
	ObserveOutboxStatusScrape(context.Context, OutboxStatusScrapeEvent)
}

type NopObserver struct{}

func (NopObserver) ObservePublish(context.Context, PublishEvent)                 {}
func (NopObserver) ObserveOutbox(context.Context, OutboxEvent)                   {}
func (NopObserver) ObserveConsume(context.Context, ConsumeEvent)                 {}
func (NopObserver) ObserveConsumeDuration(context.Context, ConsumeDurationEvent) {}
func (NopObserver) ObserveOutboxStatus(context.Context, OutboxStatusEvent)       {}
func (NopObserver) ObserveOutboxStatusScrape(context.Context, OutboxStatusScrapeEvent) {
}

func NormalizeObserver(observer Observer) Observer {
	if observer == nil {
		return NopObserver{}
	}
	return observer
}

func DefaultObserver() Observer {
	return PrometheusObserver{}
}

func ObserveConsumeDuration(ctx context.Context, observer Observer, evt ConsumeDurationEvent) {
	if observer == nil {
		return
	}
	durationObserver, ok := observer.(ConsumeDurationObserver)
	if !ok {
		return
	}
	durationObserver.ObserveConsumeDuration(ctx, evt)
}

func ObserveOutboxStatus(ctx context.Context, observer Observer, evt OutboxStatusEvent) {
	if observer == nil {
		return
	}
	statusObserver, ok := observer.(OutboxStatusObserver)
	if !ok {
		return
	}
	statusObserver.ObserveOutboxStatus(ctx, evt)
}

func ObserveOutboxStatusScrape(ctx context.Context, observer Observer, evt OutboxStatusScrapeEvent) {
	if observer == nil {
		return
	}
	scrapeObserver, ok := observer.(OutboxStatusScrapeObserver)
	if !ok {
		return
	}
	scrapeObserver.ObserveOutboxStatusScrape(ctx, evt)
}
