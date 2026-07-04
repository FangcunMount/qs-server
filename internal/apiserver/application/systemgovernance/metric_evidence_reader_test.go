package systemgovernance

import (
	"context"
	"testing"
	"time"

	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func TestMetricEvidenceReaderBuildsSortedEscapedCounterQuery(t *testing.T) {
	metrics := &recordingMetricsReader{}
	reader := NewMetricEvidenceReader(metrics)

	evidence, ok := reader.CounterIncrease(context.Background(), "queue_full", "qs_resilience_decision_total", "5m", map[string]string{
		"strategy": "memory_channel",
		"resource": "submit\"queue",
		"scope":    "submit\nqueue",
	}, time.Now())

	if !ok {
		t.Fatal("CounterIncrease() ok = false, want true")
	}
	if evidence.Name != "queue_full" || !evidence.Available {
		t.Fatalf("evidence = %#v, want available queue_full evidence", evidence)
	}
	want := `sum(increase(qs_resilience_decision_total{resource="submit\"queue",scope="submit\nqueue",strategy="memory_channel"}[5m]))`
	if len(metrics.specs) != 1 || metrics.specs[0].Query != want {
		t.Fatalf("query = %#v, want %q", metrics.specs, want)
	}
}

func TestMetricEvidenceReaderScopesResilienceDecisionLabels(t *testing.T) {
	metrics := &recordingMetricsReader{}
	reader := NewMetricEvidenceReader(metrics)
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	reader.ResilienceQueueFull(context.Background(), "collection-server", resilienceplane.QueueSnapshot{
		Component: "collection-server",
		Name:      "answersheet_submit",
		Strategy:  "memory_channel",
	}, "5m", now)
	reader.ResilienceBackpressureTimeout(context.Background(), "apiserver", resilienceplane.BackpressureSnapshot{
		Component:  "apiserver",
		Name:       "mysql",
		Dependency: "mysql",
		Strategy:   "semaphore",
	}, "5m", now)

	wantQueries := []string{
		`sum(increase(qs_resilience_decision_total{component="collection-server",kind="queue",outcome="queue_full",resource="submit_queue",scope="answersheet_submit",strategy="memory_channel"}[5m]))`,
		`sum(increase(qs_resilience_decision_total{component="apiserver",kind="backpressure",outcome="backpressure_timeout",resource="downstream",scope="mysql",strategy="semaphore"}[5m]))`,
	}
	if len(metrics.specs) != len(wantQueries) {
		t.Fatalf("metrics specs = %#v, want %d specs", metrics.specs, len(wantQueries))
	}
	if metrics.specs[0].Name != "queue_full_collection-server_answersheet_submit" {
		t.Fatalf("queue evidence name = %q, want legacy display name", metrics.specs[0].Name)
	}
	for i, want := range wantQueries {
		if metrics.specs[i].Query != want {
			t.Fatalf("query[%d] = %q, want %q", i, metrics.specs[i].Query, want)
		}
	}
}

type unavailableMetricsReader struct{}

func (unavailableMetricsReader) Query(_ context.Context, spec govprom.QuerySpec, _ time.Time) govprom.MetricResult {
	return govprom.MetricResult{
		Name:      spec.Name,
		Window:    spec.Window,
		Unit:      spec.Unit,
		Available: false,
		Reason:    "prometheus unavailable",
	}
}
