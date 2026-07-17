package systemgovernance

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

func TestResilienceProjectionBuildsSummaryRowsSignalsAndMetrics(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	metrics := &recordingMetricsReader{}
	snapshot := resilience.RuntimeSnapshot{
		GeneratedAt: now,
		Component:   "collection-server",
		Summary:     resilience.RuntimeSummary{Ready: false, CapabilityCount: 4, DegradedCount: 1},
		Queues: []resilience.QueueSnapshot{{
			Component:         "collection-server",
			Name:              "answersheet_submit",
			Strategy:          "memory_channel",
			Depth:             95,
			Capacity:          100,
			StatusCounts:      map[string]int{"pending": 95},
			LifecycleBoundary: "process_memory_no_drain",
		}},
		Backpressure: []resilience.BackpressureSnapshot{{
			Component:   "collection-server",
			Name:        "mysql",
			Dependency:  "mysql",
			Strategy:    "semaphore",
			Enabled:     true,
			InFlight:    8,
			MaxInflight: 10,
		}},
		RateLimits: []resilience.CapabilitySnapshot{{
			Name:       "api_global",
			Kind:       "rate_limit",
			Strategy:   "token_bucket",
			Configured: true,
			Degraded:   true,
			Reason:     "redis degraded",
		}},
	}

	projection := NewResilienceProjector(metrics).Evaluate(context.Background(), map[string]ComponentResilience{
		"collection-server": {Available: true, Snapshot: &snapshot},
		"worker":            {Available: false, Reason: "connection refused"},
	}, "5m", now)

	if projection.Summary.ComponentCount != 2 ||
		projection.Summary.UnavailableComponentCount != 1 ||
		projection.Summary.NotReadyComponentCount != 1 ||
		projection.Summary.QueueCount != 1 ||
		projection.Summary.CriticalQueueCount != 1 ||
		projection.Summary.BackpressureCount != 1 ||
		projection.Summary.WarningBackpressureCount != 1 ||
		projection.Summary.DegradedCapabilityCount != 1 {
		t.Fatalf("summary = %#v, want component/queue/backpressure/capability counts", projection.Summary)
	}
	if len(projection.QueueRows) != 1 || projection.QueueRows[0].Severity != SeverityCritical || projection.QueueRows[0].Utilization != 0.95 {
		t.Fatalf("queue rows = %#v, want one critical 95%% row", projection.QueueRows)
	}
	if len(projection.BackpressureRows) != 1 || projection.BackpressureRows[0].Severity != SeverityWarning || projection.BackpressureRows[0].Utilization != 0.8 {
		t.Fatalf("backpressure rows = %#v, want one warning 80%% row", projection.BackpressureRows)
	}
	if len(projection.CapabilityRows) != 1 || projection.CapabilityRows[0].Kind != "rate_limit" || projection.CapabilityRows[0].Severity != SeverityWarning {
		t.Fatalf("capability rows = %#v, want degraded rate_limit row", projection.CapabilityRows)
	}
	statuses := map[string]bool{}
	for _, signal := range projection.Signals {
		statuses[signal.Status] = true
	}
	for _, status := range []string{"component_unavailable", "not_ready", "queue_utilization_critical", "backpressure_utilization"} {
		if !statuses[status] {
			t.Fatalf("signals = %#v, missing %s signal", projection.Signals, status)
		}
	}
	if len(metrics.specs) != 2 {
		t.Fatalf("metrics specs = %#v, want queue and backpressure evidence", metrics.specs)
	}
}

func TestResilienceProjectionKeepsPrometheusFailureAsMetricEvidence(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	snapshot := resilience.NewRuntimeSnapshot("apiserver", now)
	snapshot.Queues = []resilience.QueueSnapshot{{
		Component: "apiserver",
		Name:      "submit",
		Strategy:  "memory_channel",
		Depth:     80,
		Capacity:  100,
	}}

	projection := NewResilienceProjector(unavailableMetricsReader{}).Evaluate(context.Background(), map[string]ComponentResilience{
		"apiserver": {Available: true, Snapshot: &snapshot},
	}, "5m", now)

	if len(projection.QueueRows) != 1 || len(projection.QueueRows[0].MetricEvidence) != 1 {
		t.Fatalf("queue rows = %#v, want one row with metric evidence", projection.QueueRows)
	}
	evidence := projection.QueueRows[0].MetricEvidence[0]
	if evidence.Available || evidence.Reason != "prometheus unavailable" {
		t.Fatalf("metric evidence = %#v, want unavailable evidence with reason", evidence)
	}
	if len(projection.Signals) != 1 || len(projection.Signals[0].MetricEvidence) != 1 {
		t.Fatalf("signals = %#v, want warning signal carrying unavailable metric evidence", projection.Signals)
	}
}

func TestResilienceProjectionFlagsQueueUtilizationWithoutMetrics(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	snapshot := resilience.NewRuntimeSnapshot("collection-server", now)
	snapshot.Queues = []resilience.QueueSnapshot{
		{Name: "submit", Depth: 90, Capacity: 100},
	}

	projection := NewResilienceProjector(nil).Evaluate(context.Background(), map[string]ComponentResilience{
		"collection-server": {Available: true, Snapshot: &snapshot},
	}, "5m", now)

	if len(projection.Signals) != 1 || projection.Signals[0].Severity != SeverityCritical {
		t.Fatalf("signals = %#v, want one critical queue utilization signal", projection.Signals)
	}
}
