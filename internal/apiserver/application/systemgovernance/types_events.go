package systemgovernance

import (
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

// EventsView 暴露event/outbox governance detail。
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

// EventTypeStatusGroup 分组按事件类型 积压 行 用于 一个outbox 存储。
type EventTypeStatusGroup struct {
	Store   string                             `json:"store"`
	Buckets []outboxport.EventTypeStatusBucket `json:"buckets"`
	Error   string                             `json:"error,omitempty"`
}

// EventDrainSummary 汇总outbox drain 健康度 用于 workbench。
type EventDrainSummary struct {
	OutboxCount             int     `json:"outbox_count"`
	DegradedReaderCount     int     `json:"degraded_reader_count"`
	PendingCount            int64   `json:"pending_count"`
	FailedCount             int64   `json:"failed_count"`
	OldestPendingAgeSeconds float64 `json:"oldest_pending_age_seconds"`
	StaleEventTypeCount     int     `json:"stale_event_type_count"`
	ReaderErrorCount        int     `json:"reader_error_count"`
}

// EventOutboxRow 是面向 UI outbox drain 行 派生 从 状态 buckets。
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

// EventTypeRow 是面向 UI 事件类型积压 行。
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

// EventTypeStatusSource 暴露按事件类型 积压 用于 一个outbox 存储。
type EventTypeStatusSource struct {
	Store  string
	Reader outboxport.EventTypeStatusReader
}
