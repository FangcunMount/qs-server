package eventing

import (
	"context"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

type StatusService interface {
	GetStatus(ctx context.Context) (*StatusSnapshot, error)
}

type StatusServiceOptions struct {
	Catalog         *eventcatalog.Catalog
	Registry        *eventcatalog.EffectiveRegistry
	Outboxes        []NamedOutboxStatusReader
	RuntimeSnapshot func() RuntimeStatusSnapshot
	Now             func() time.Time
}

type RuntimeStatusSnapshot struct {
	Profiles  map[eventcatalog.OutboxProfile]ProfileRuntimeStatus
	Consumers map[string]ConsumerRuntimeStatus
}

type ProfileRuntimeStatus struct {
	Running           bool
	RelayEnabled      bool
	ReconcilerEnabled bool
	ImmediateEnabled  bool
}

type ConsumerRuntimeStatus struct {
	Topic     string
	Enabled   bool
	Healthy   bool
	LastError string
}

type NamedOutboxStatusReader struct {
	Name   string
	Reader OutboxStatusReader
}

type StatusSnapshot struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Catalog     CatalogSummary    `json:"catalog"`
	Outboxes    []OutboxSummary   `json:"outboxes"`
	Events      []EventSummary    `json:"events,omitempty"`
	Profiles    []ProfileSummary  `json:"profiles,omitempty"`
	Consumers   []ConsumerSummary `json:"consumers,omitempty"`
}

type EventSummary struct {
	Type        string                        `json:"type"`
	Owner       string                        `json:"owner"`
	Delivery    eventcatalog.DeliveryClass    `json:"delivery"`
	Profile     eventcatalog.OutboxProfile    `json:"profile,omitempty"`
	Immediate   bool                          `json:"immediate"`
	Priority    eventcatalog.Priority         `json:"priority,omitempty"`
	Handler     string                        `json:"handler"`
	Idempotency string                        `json:"idempotency"`
	Settlement  eventcatalog.SettlementPolicy `json:"settlement"`
}

type ProfileSummary struct {
	Name                eventcatalog.OutboxProfile `json:"name"`
	EventCount          int                        `json:"event_count"`
	ImmediateEventTypes []string                   `json:"immediate_event_types,omitempty"`
	Running             bool                       `json:"running"`
	RelayEnabled        bool                       `json:"relay_enabled"`
	ReconcilerEnabled   bool                       `json:"reconciler_enabled"`
	ImmediateEnabled    bool                       `json:"immediate_enabled"`
}

type ConsumerSummary struct {
	ID         string                        `json:"id"`
	EventType  string                        `json:"event_type"`
	Runtime    string                        `json:"runtime"`
	Topic      string                        `json:"topic"`
	Channel    string                        `json:"channel"`
	Enabled    bool                          `json:"enabled"`
	Healthy    bool                          `json:"healthy"`
	LastError  string                        `json:"last_error,omitempty"`
	Settlement eventcatalog.SettlementPolicy `json:"settlement"`
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
	catalog         *eventcatalog.Catalog
	registry        *eventcatalog.EffectiveRegistry
	outboxes        []NamedOutboxStatusReader
	runtimeSnapshot func() RuntimeStatusSnapshot
	now             func() time.Time
}

func NewStatusService(opts StatusServiceOptions) StatusService {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &statusService{
		catalog:         opts.Catalog,
		registry:        opts.Registry,
		outboxes:        append([]NamedOutboxStatusReader(nil), opts.Outboxes...),
		runtimeSnapshot: opts.RuntimeSnapshot,
		now:             opts.Now,
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
	snapshot.Events, snapshot.Profiles, snapshot.Consumers = summarizeEffectiveRegistry(s.registry)
	if s.runtimeSnapshot != nil {
		applyRuntimeStatus(snapshot, s.runtimeSnapshot())
	}
	snapshot.Outboxes = make([]OutboxSummary, 0, len(s.outboxes))
	for _, outbox := range s.outboxes {
		snapshot.Outboxes = append(snapshot.Outboxes, readOutboxSummary(ctx, outbox, now))
	}
	return snapshot, nil
}

func applyRuntimeStatus(snapshot *StatusSnapshot, runtime RuntimeStatusSnapshot) {
	for i := range snapshot.Profiles {
		status := runtime.Profiles[snapshot.Profiles[i].Name]
		snapshot.Profiles[i].Running = status.Running
		snapshot.Profiles[i].RelayEnabled = status.RelayEnabled
		snapshot.Profiles[i].ReconcilerEnabled = status.ReconcilerEnabled
		snapshot.Profiles[i].ImmediateEnabled = status.ImmediateEnabled
	}
	for i := range snapshot.Consumers {
		status := runtime.Consumers[snapshot.Consumers[i].ID]
		snapshot.Consumers[i].Topic = status.Topic
		snapshot.Consumers[i].Enabled = status.Enabled
		snapshot.Consumers[i].Healthy = status.Healthy
		snapshot.Consumers[i].LastError = status.LastError
	}
}

func summarizeEffectiveRegistry(registry *eventcatalog.EffectiveRegistry) ([]EventSummary, []ProfileSummary, []ConsumerSummary) {
	if registry == nil {
		return nil, nil, nil
	}
	events := make([]EventSummary, 0)
	consumers := make([]ConsumerSummary, 0)
	profileCounts := map[eventcatalog.OutboxProfile]int{}
	for _, evt := range registry.Snapshot() {
		events = append(events, EventSummary{
			Type: evt.Type, Owner: evt.Owner, Delivery: evt.Delivery, Profile: evt.OutboxProfile,
			Immediate: evt.Immediate, Priority: evt.Priority, Handler: evt.PrimaryHandler,
			Idempotency: evt.IdempotencyPolicy, Settlement: evt.SettlementPolicy,
		})
		if evt.OutboxProfile != eventcatalog.OutboxProfileNone {
			profileCounts[evt.OutboxProfile]++
		}
		for _, consumer := range evt.AdditionalConsumers {
			consumers = append(consumers, ConsumerSummary{ID: consumer.ID, EventType: evt.Type, Runtime: consumer.Runtime, Channel: consumer.Channel, Settlement: consumer.SettlementPolicy})
		}
	}
	profiles := make([]ProfileSummary, 0, len(profileCounts))
	for _, profile := range []eventcatalog.OutboxProfile{eventcatalog.OutboxProfileMongoDomain, eventcatalog.OutboxProfileAssessmentMySQL} {
		if count := profileCounts[profile]; count > 0 {
			profiles = append(profiles, ProfileSummary{Name: profile, EventCount: count, ImmediateEventTypes: registry.ImmediateTypes(profile)})
		}
	}
	return events, profiles, consumers
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
