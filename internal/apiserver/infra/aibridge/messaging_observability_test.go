package aibridge

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMQObservationFailureDoesNotReportEmptyQueue(t *testing.T) {
	// Constructor/Describe are inert; no pool, client, background task or registration.
	collector := newMessagingCollector(nil)
	registry := prometheus.NewRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0].GetName() != "qs_server_ai_mq_observation_available" || families[0].Metric[0].GetGauge().GetValue() != 0 {
		t.Fatal("unavailable storage emitted empty/partial state", families)
	}
	if !registry.Unregister(collector) {
		t.Fatal("collector registration could not be released")
	}
	families, err = registry.Gather()
	if err != nil || len(families) != 0 {
		t.Fatal("stopped observer remained registered", err, families)
	}
}
