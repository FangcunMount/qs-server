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
