package eventing

import (
	"context"
	"errors"
	"testing"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

type fakeStatusReader struct {
	snapshot outboxport.StatusSnapshot
	err      error
}

func (r fakeStatusReader) OutboxStatusSnapshot(context.Context, time.Time) (outboxport.StatusSnapshot, error) {
	return r.snapshot, r.err
}

func TestStatusServiceReturnsCatalogAndOutboxSnapshots(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load events config: %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	service := NewStatusService(StatusServiceOptions{
		Catalog: eventcatalog.NewCatalog(cfg),
		Outboxes: []NamedOutboxStatusReader{
			{
				Name: "mysql",
				Reader: fakeStatusReader{snapshot: outboxport.StatusSnapshot{
					Store:       "mysql",
					GeneratedAt: now,
					Buckets:     []outboxport.StatusBucket{{Status: "pending", Count: 2}},
				}},
			},
		},
		Now: func() time.Time { return now },
	})

	snapshot, err := service.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if snapshot.Catalog.EventCount == 0 || snapshot.Catalog.TopicCount == 0 {
		t.Fatalf("catalog summary = %#v, want non-empty", snapshot.Catalog)
	}
	if snapshot.Catalog.BestEffortCount == 0 || snapshot.Catalog.DurableOutboxCount == 0 {
		t.Fatalf("catalog delivery summary = %#v, want both delivery classes", snapshot.Catalog)
	}
	if len(snapshot.Outboxes) != 1 || snapshot.Outboxes[0].Degraded {
		t.Fatalf("outboxes = %#v, want one healthy outbox", snapshot.Outboxes)
	}
	if snapshot.Outboxes[0].Buckets[0].Count != 2 {
		t.Fatalf("bucket count = %d, want 2", snapshot.Outboxes[0].Buckets[0].Count)
	}
}

func TestStatusServiceMarksSingleOutboxDegraded(t *testing.T) {
	service := NewStatusService(StatusServiceOptions{
		Outboxes: []NamedOutboxStatusReader{
			{Name: "mysql", Reader: fakeStatusReader{err: errors.New("db unavailable")}},
		},
	})

	snapshot, err := service.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(snapshot.Outboxes) != 1 || !snapshot.Outboxes[0].Degraded {
		t.Fatalf("outboxes = %#v, want degraded outbox", snapshot.Outboxes)
	}
	if snapshot.Outboxes[0].Error == "" {
		t.Fatalf("degraded outbox should include error")
	}
}
