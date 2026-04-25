package eventobservability

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	eventPublishTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_event_publish_total",
			Help: "Total event publish outcomes.",
		},
		[]string{"source", "mode", "topic", "event_type", "outcome"},
	)
	eventOutboxTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_event_outbox_total",
			Help: "Total event outbox delivery outcomes.",
		},
		[]string{"relay", "topic", "event_type", "outcome"},
	)
	eventConsumeTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_event_consume_total",
			Help: "Total event consume outcomes.",
		},
		[]string{"service", "topic", "event_type", "outcome"},
	)
	eventConsumeDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "qs_event_consume_duration_seconds",
			Help:    "Event worker dispatch and settlement duration.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "topic", "event_type", "outcome"},
	)
	eventOutboxBacklog = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "qs_event_outbox_backlog",
			Help: "Current unfinished event outbox backlog by store and status.",
		},
		[]string{"store", "status"},
	)
	eventOutboxOldestAge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "qs_event_outbox_oldest_age_seconds",
			Help: "Age in seconds of the oldest unfinished event outbox row by store and status.",
		},
		[]string{"store", "status"},
	)
	eventOutboxStatusScrapeTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_event_outbox_status_scrape_total",
			Help: "Total event outbox status scrape outcomes.",
		},
		[]string{"store", "outcome"},
	)
)

type PrometheusObserver struct{}

func NewPrometheusObserver() *PrometheusObserver {
	return &PrometheusObserver{}
}

func (PrometheusObserver) ObservePublish(_ context.Context, evt PublishEvent) {
	eventPublishTotal.WithLabelValues(evt.Source, evt.Mode, evt.Topic, evt.EventType, evt.Outcome.String()).Inc()
}

func (PrometheusObserver) ObserveOutbox(_ context.Context, evt OutboxEvent) {
	eventOutboxTotal.WithLabelValues(evt.Relay, evt.Topic, evt.EventType, evt.Outcome.String()).Inc()
}

func (PrometheusObserver) ObserveConsume(_ context.Context, evt ConsumeEvent) {
	eventConsumeTotal.WithLabelValues(evt.Service, evt.Topic, evt.EventType, evt.Outcome.String()).Inc()
}

func (PrometheusObserver) ObserveConsumeDuration(_ context.Context, evt ConsumeDurationEvent) {
	duration := evt.Duration.Seconds()
	if duration < 0 {
		duration = 0
	}
	eventConsumeDuration.WithLabelValues(evt.Service, evt.Topic, evt.EventType, evt.Outcome.String()).Observe(duration)
}

func (PrometheusObserver) ObserveOutboxStatus(_ context.Context, evt OutboxStatusEvent) {
	eventOutboxBacklog.WithLabelValues(evt.Store, evt.Status).Set(float64(evt.Count))
	age := evt.OldestAgeSeconds
	if age < 0 {
		age = 0
	}
	eventOutboxOldestAge.WithLabelValues(evt.Store, evt.Status).Set(age)
}

func (PrometheusObserver) ObserveOutboxStatusScrape(_ context.Context, evt OutboxStatusScrapeEvent) {
	eventOutboxStatusScrapeTotal.WithLabelValues(evt.Store, evt.Outcome.String()).Inc()
}
