package systemgovernance

import (
	"context"
	"testing"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
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
