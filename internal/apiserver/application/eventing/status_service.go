package eventing

import (
	"context"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

type StatusService interface {
	GetStatus(ctx context.Context) (*StatusSnapshot, error)
}

type StatusServiceOptions struct {
	Catalog  *eventcatalog.Catalog
	Outboxes []NamedOutboxStatusReader
	Now      func() time.Time
}

type NamedOutboxStatusReader struct {
	Name   string
	Reader OutboxStatusReader
}

type StatusSnapshot struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Catalog     CatalogSummary  `json:"catalog"`
	Outboxes    []OutboxSummary `json:"outboxes"`
}

type CatalogSummary struct {
	TopicCount         int `json:"topic_count"`
	EventCount         int `json:"event_count"`
	BestEffortCount    int `json:"best_effort_count"`
	DurableOutboxCount int `json:"durable_outbox_count"`
}

type OutboxSummary struct {
	Name        string                    `json:"name"`
	Store       string                    `json:"store"`
	Degraded    bool                      `json:"degraded"`
	Error       string                    `json:"error,omitempty"`
	GeneratedAt time.Time                 `json:"generated_at,omitempty"`
	Buckets     []outboxport.StatusBucket `json:"buckets,omitempty"`
}

type statusService struct {
	catalog  *eventcatalog.Catalog
	outboxes []NamedOutboxStatusReader
	now      func() time.Time
}

func NewStatusService(opts StatusServiceOptions) StatusService {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &statusService{
		catalog:  opts.Catalog,
		outboxes: append([]NamedOutboxStatusReader(nil), opts.Outboxes...),
		now:      opts.Now,
	}
}

func (s *statusService) GetStatus(ctx context.Context) (*StatusSnapshot, error) {
	now := time.Now()
	if s != nil && s.now != nil {
		now = s.now()
	}
	snapshot := &StatusSnapshot{
		GeneratedAt: now,
	}
	if s == nil {
		return snapshot, nil
	}
	snapshot.Catalog = summarizeCatalog(s.catalog)
	snapshot.Outboxes = make([]OutboxSummary, 0, len(s.outboxes))
	for _, outbox := range s.outboxes {
		snapshot.Outboxes = append(snapshot.Outboxes, readOutboxSummary(ctx, outbox, now))
	}
	return snapshot, nil
}

func summarizeCatalog(catalog *eventcatalog.Catalog) CatalogSummary {
	if catalog == nil {
		return CatalogSummary{}
	}
	cfg := catalog.Config()
	if cfg == nil {
		return CatalogSummary{}
	}
	summary := CatalogSummary{
		TopicCount: len(cfg.Topics),
		EventCount: len(cfg.Events),
	}
	for eventType := range cfg.Events {
		delivery, ok := catalog.GetDeliveryClass(eventType)
		if !ok {
			continue
		}
		switch delivery {
		case eventcatalog.DeliveryClassBestEffort:
			summary.BestEffortCount++
		case eventcatalog.DeliveryClassDurableOutbox:
			summary.DurableOutboxCount++
		}
	}
	return summary
}

func readOutboxSummary(ctx context.Context, outbox NamedOutboxStatusReader, now time.Time) OutboxSummary {
	summary := OutboxSummary{Name: outbox.Name, Store: outbox.Name}
	if outbox.Reader == nil {
		summary.Degraded = true
		summary.Error = "outbox status reader unavailable"
		return summary
	}
	snapshot, err := outbox.Reader.OutboxStatusSnapshot(ctx, now)
	if snapshot.Store != "" {
		summary.Store = snapshot.Store
	}
	if err != nil {
		summary.Degraded = true
		summary.Error = err.Error()
		return summary
	}
	summary.GeneratedAt = snapshot.GeneratedAt
	summary.Buckets = append([]outboxport.StatusBucket(nil), snapshot.Buckets...)
	return summary
}
