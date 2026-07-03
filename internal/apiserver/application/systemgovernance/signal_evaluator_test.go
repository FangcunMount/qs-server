package systemgovernance

import (
	"context"
	"testing"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func TestEvaluateEventsFlagsPendingBacklog(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	signals := NewEvaluator(nil).EvaluateEvents(context.Background(), &appEventing.StatusSnapshot{
		Outboxes: []appEventing.OutboxSummary{
			{
				Name: "mysql",
				Buckets: []outboxport.StatusBucket{
					{Status: "pending", Count: 3, OldestAgeSeconds: 400},
				},
			},
		},
	}, nil, "5m", now)
	if len(signals) != 1 || signals[0].Severity != SeverityWarning {
		t.Fatalf("signals = %#v, want one warning pending_stale signal", signals)
	}
}

func TestEvaluateResilienceFlagsQueueUtilization(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	snapshot := resilienceplane.NewRuntimeSnapshot("collection-server", now)
	snapshot.Queues = []resilienceplane.QueueSnapshot{
		{Name: "submit", Depth: 90, Capacity: 100},
	}
	signals := NewEvaluator(nil).EvaluateResilience(context.Background(), snapshot, nil, "5m", now)
	if len(signals) != 1 || signals[0].Severity != SeverityCritical {
		t.Fatalf("signals = %#v, want one critical queue utilization signal", signals)
	}
}

func TestEvaluateResilienceScopesMetricEvidenceToControlPoint(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	metrics := &recordingMetricsReader{}
	snapshot := resilienceplane.NewRuntimeSnapshot("apiserver", now)
	snapshot.Queues = []resilienceplane.QueueSnapshot{
		{
			Component: "collection-server",
			Name:      "answersheet_submit",
			Strategy:  "memory_channel",
			Depth:     75,
			Capacity:  100,
		},
	}
	snapshot.Backpressure = []resilienceplane.BackpressureSnapshot{
		{
			Component:   "apiserver",
			Name:        "mysql",
			Dependency:  "mysql",
			Strategy:    "semaphore",
			InFlight:    8,
			MaxInflight: 10,
		},
	}

	signals := NewEvaluator(metrics).EvaluateResilience(context.Background(), snapshot, nil, "5m", now)
	if len(signals) != 2 {
		t.Fatalf("signals = %#v, want queue and backpressure warning signals", signals)
	}
	if len(metrics.specs) != 2 {
		t.Fatalf("metrics specs = %#v, want two scoped queries", metrics.specs)
	}
	wantQueries := []string{
		`sum(increase(qs_resilience_decision_total{component="collection-server",kind="queue",outcome="queue_full",resource="submit_queue",scope="answersheet_submit",strategy="memory_channel"}[5m]))`,
		`sum(increase(qs_resilience_decision_total{component="apiserver",kind="backpressure",outcome="backpressure_timeout",resource="downstream",scope="mysql",strategy="semaphore"}[5m]))`,
	}
	for i, want := range wantQueries {
		if metrics.specs[i].Query != want {
			t.Fatalf("query[%d] = %q, want %q", i, metrics.specs[i].Query, want)
		}
	}
}

type recordingMetricsReader struct {
	specs []govprom.QuerySpec
}

func (r *recordingMetricsReader) Query(_ context.Context, spec govprom.QuerySpec, _ time.Time) govprom.MetricResult {
	r.specs = append(r.specs, spec)
	value := 1.0
	return govprom.MetricResult{
		Name:      spec.Name,
		Window:    spec.Window,
		Unit:      spec.Unit,
		Value:     &value,
		Available: true,
	}
}
