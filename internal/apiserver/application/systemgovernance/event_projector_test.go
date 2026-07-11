package systemgovernance

import (
	"context"
	"strings"
	"testing"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

func TestEventDrainProjectionBuildsRowsSummaryAndScopedMetrics(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	oldest := now.Add(-10 * time.Minute)
	metrics := &recordingMetricsReader{}

	projection := NewEventDrainEvaluator(metrics).Evaluate(context.Background(), &appEventing.StatusSnapshot{
		Outboxes: []appEventing.OutboxSummary{
			{
				Name:  "mysql",
				Store: "mysql",
				Buckets: []outboxport.StatusBucket{
					{Status: "pending", Count: 12, OldestAgeSeconds: 600},
					{Status: "failed", Count: 2},
				},
			},
		},
	}, []EventTypeStatusGroup{
		{
			Store: "mysql",
			Buckets: []outboxport.EventTypeStatusBucket{
				{EventType: "evaluation.requested", Status: "pending", Count: 9, OldestCreatedAt: &oldest},
			},
		},
	}, "5m", now)

	if projection.Summary.PendingCount != 12 || projection.Summary.FailedCount != 2 || projection.Summary.StaleEventTypeCount != 1 {
		t.Fatalf("summary = %#v, want pending/failed/stale counts", projection.Summary)
	}
	if len(projection.OutboxRows) != 1 || projection.OutboxRows[0].Severity != SeverityCritical {
		t.Fatalf("outbox rows = %#v, want one critical row", projection.OutboxRows)
	}
	if len(projection.EventTypeRows) != 1 || projection.EventTypeRows[0].EventType != "evaluation.requested" {
		t.Fatalf("event type rows = %#v, want assessment.submitted row", projection.EventTypeRows)
	}
	if len(projection.Signals) < 2 {
		t.Fatalf("signals = %#v, want outbox and event type signals", projection.Signals)
	}
	var sawStoreScoped, sawTypeScoped bool
	for _, spec := range metrics.specs {
		if strings.Contains(spec.Query, `store="mysql"`) && strings.Contains(spec.Query, `status="pending"`) {
			sawStoreScoped = true
		}
		if strings.Contains(spec.Query, `event_type="evaluation.requested"`) {
			sawTypeScoped = true
		}
	}
	if !sawStoreScoped || !sawTypeScoped {
		t.Fatalf("metric specs = %#v, want store and event_type scoped queries", metrics.specs)
	}
}

func TestEventDrainProjectionMarksEventTypeReaderError(t *testing.T) {
	projection := NewEventDrainEvaluator(nil).Evaluate(context.Background(), nil, []EventTypeStatusGroup{
		{Store: "mongo", Error: "reader unavailable"},
	}, "5m", time.Now())
	if projection.Summary.ReaderErrorCount != 1 {
		t.Fatalf("reader errors = %d, want 1", projection.Summary.ReaderErrorCount)
	}
	if len(projection.EventTypeRows) != 1 || !projection.EventTypeRows[0].Degraded {
		t.Fatalf("event type rows = %#v, want degraded reader row", projection.EventTypeRows)
	}
	if len(projection.Signals) != 1 || projection.Signals[0].Status != "event_type_reader_error" {
		t.Fatalf("signals = %#v, want reader error signal", projection.Signals)
	}
}

func TestEventDrainProjectionFlagsPendingBacklogWarning(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	projection := NewEventDrainEvaluator(nil).Evaluate(context.Background(), &appEventing.StatusSnapshot{
		Outboxes: []appEventing.OutboxSummary{
			{
				Name: "mysql",
				Buckets: []outboxport.StatusBucket{
					{Status: "pending", Count: 3, OldestAgeSeconds: 400},
				},
			},
		},
	}, nil, "5m", now)

	if len(projection.Signals) != 1 || projection.Signals[0].Severity != SeverityWarning {
		t.Fatalf("signals = %#v, want one warning pending_stale signal", projection.Signals)
	}
}
