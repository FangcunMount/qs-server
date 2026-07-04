package systemgovernance

import (
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

// EventsView exposes event/outbox governance detail.
type EventsView struct {
	GeneratedAt time.Time                   `json:"generated_at"`
	Window      string                      `json:"window"`
	Metrics     MetricsSummary              `json:"metrics"`
	Signals     []Signal                    `json:"signals"`
	Snapshot    *appEventing.StatusSnapshot `json:"snapshot,omitempty"`
	EventTypes  []EventTypeStatusGroup      `json:"event_types,omitempty"`
	Summary     EventDrainSummary           `json:"summary"`
	OutboxRows  []EventOutboxRow            `json:"outbox_rows,omitempty"`
	TypeRows    []EventTypeRow              `json:"event_type_rows,omitempty"`
}

// EventTypeStatusGroup groups per-event-type backlog rows for one outbox store.
type EventTypeStatusGroup struct {
	Store   string                             `json:"store"`
	Buckets []outboxport.EventTypeStatusBucket `json:"buckets"`
	Error   string                             `json:"error,omitempty"`
}

// EventDrainSummary summarizes outbox drain health for the workbench.
type EventDrainSummary struct {
	OutboxCount             int     `json:"outbox_count"`
	DegradedReaderCount     int     `json:"degraded_reader_count"`
	PendingCount            int64   `json:"pending_count"`
	FailedCount             int64   `json:"failed_count"`
	OldestPendingAgeSeconds float64 `json:"oldest_pending_age_seconds"`
	StaleEventTypeCount     int     `json:"stale_event_type_count"`
	ReaderErrorCount        int     `json:"reader_error_count"`
}

// EventOutboxRow is a UI-ready outbox drain row derived from status buckets.
type EventOutboxRow struct {
	Name                    string           `json:"name"`
	Store                   string           `json:"store"`
	Degraded                bool             `json:"degraded"`
	PendingCount            int64            `json:"pending_count"`
	FailedCount             int64            `json:"failed_count"`
	PublishingCount         int64            `json:"publishing_count"`
	OldestPendingAgeSeconds float64          `json:"oldest_pending_age_seconds"`
	Severity                Severity         `json:"severity"`
	Reason                  string           `json:"reason,omitempty"`
	MetricEvidence          []MetricEvidence `json:"metric_evidence,omitempty"`
}

// EventTypeRow is a UI-ready event type backlog row.
type EventTypeRow struct {
	Store            string           `json:"store"`
	EventType        string           `json:"event_type"`
	PendingCount     int64            `json:"pending_count"`
	FailedCount      int64            `json:"failed_count"`
	OldestAgeSeconds float64          `json:"oldest_age_seconds"`
	Severity         Severity         `json:"severity"`
	Degraded         bool             `json:"degraded"`
	Reason           string           `json:"reason,omitempty"`
	MetricEvidence   []MetricEvidence `json:"metric_evidence,omitempty"`
}

// EventTypeStatusSource exposes per-event-type backlog for one outbox store.
type EventTypeStatusSource struct {
	Store  string
	Reader outboxport.EventTypeStatusReader
}
