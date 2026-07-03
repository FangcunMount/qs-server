package systemgovernance

import (
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// Domain identifies a governance concern area.
type Domain string

const (
	DomainEvents     Domain = "events"
	DomainCache      Domain = "cache"
	DomainResilience Domain = "resilience"
	DomainActions    Domain = "actions"
)

// Severity ranks diagnostic signals.
type Severity string

const (
	SeverityHealthy  Severity = "healthy"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Signal is a bounded diagnostic item for the governance workbench.
type Signal struct {
	ID             string                 `json:"id"`
	Domain         Domain                 `json:"domain"`
	Severity       Severity               `json:"severity"`
	Status         string                 `json:"status"`
	Title          string                 `json:"title"`
	Evidence       map[string]interface{} `json:"evidence,omitempty"`
	MetricEvidence []MetricEvidence       `json:"metric_evidence,omitempty"`
	DashboardKey   string                 `json:"dashboard_key,omitempty"`
	ActionIDs      []string               `json:"action_ids,omitempty"`
}

// MetricEvidence carries a single near-window metric observation.
type MetricEvidence struct {
	Name      string   `json:"name"`
	Window    string   `json:"window"`
	Value     *float64 `json:"value,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Available bool     `json:"available"`
	Reason    string   `json:"reason,omitempty"`
}

// ActionDescriptor describes a governance command exposed to operators.
type ActionDescriptor struct {
	ID                   string                 `json:"id"`
	Domain               Domain                 `json:"domain"`
	Label                string                 `json:"label"`
	RiskLevel            string                 `json:"risk_level"`
	Enabled              bool                   `json:"enabled"`
	Planned              bool                   `json:"planned"`
	RequiresConfirmation bool                   `json:"requires_confirmation"`
	InputSchema          map[string]interface{} `json:"input_schema,omitempty"`
}

// MetricsSummary aggregates Prometheus availability for a view.
type MetricsSummary struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// OverviewResponse is the top-level governance workbench snapshot.
type OverviewResponse struct {
	GeneratedAt     time.Time                `json:"generated_at"`
	Window          string                   `json:"window"`
	OverallSeverity Severity                 `json:"overall_severity"`
	Metrics         MetricsSummary           `json:"metrics"`
	Signals         []Signal                 `json:"signals"`
	Domains         map[Domain]DomainSummary `json:"domains"`
}

// DomainSummary summarizes one domain in the overview.
type DomainSummary struct {
	Severity    Severity `json:"severity"`
	SignalCount int      `json:"signal_count"`
}

// EventsView exposes event/outbox governance detail.
type EventsView struct {
	GeneratedAt time.Time                   `json:"generated_at"`
	Window      string                      `json:"window"`
	Metrics     MetricsSummary              `json:"metrics"`
	Signals     []Signal                    `json:"signals"`
	Snapshot    *appEventing.StatusSnapshot `json:"snapshot,omitempty"`
	EventTypes  []EventTypeStatusGroup      `json:"event_types,omitempty"`
}

// EventTypeStatusGroup groups per-event-type backlog rows for one outbox store.
type EventTypeStatusGroup struct {
	Store   string                             `json:"store"`
	Buckets []outboxport.EventTypeStatusBucket `json:"buckets"`
	Error   string                             `json:"error,omitempty"`
}

// CacheView exposes cache governance detail.
type CacheView struct {
	GeneratedAt time.Time                `json:"generated_at"`
	Window      string                   `json:"window"`
	Metrics     MetricsSummary           `json:"metrics"`
	Signals     []Signal                 `json:"signals"`
	Snapshot    *cachegov.StatusSnapshot `json:"snapshot,omitempty"`
}

// ResilienceView aggregates resilience snapshots across components.
type ResilienceView struct {
	GeneratedAt time.Time                      `json:"generated_at"`
	Window      string                         `json:"window"`
	Metrics     MetricsSummary                 `json:"metrics"`
	Signals     []Signal                       `json:"signals"`
	Components  map[string]ComponentResilience `json:"components"`
}

// ComponentResilience holds one component resilience payload with fetch metadata.
type ComponentResilience struct {
	Available bool                             `json:"available"`
	Reason    string                           `json:"reason,omitempty"`
	Snapshot  *resilienceplane.RuntimeSnapshot `json:"snapshot,omitempty"`
}

// ComponentCache holds one component cache/redis payload with fetch metadata.
type ComponentCache struct {
	Available bool                           `json:"available"`
	Reason    string                         `json:"reason,omitempty"`
	Snapshot  *observability.RuntimeSnapshot `json:"snapshot,omitempty"`
}

// ActionsView lists governance commands.
type ActionsView struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Actions     []ActionDescriptor `json:"actions"`
}

// ActionRunRequest is the body for POST /actions/:action_id/runs.
type ActionRunRequest struct {
	Confirm bool                   `json:"confirm"`
	Input   map[string]interface{} `json:"input,omitempty"`
}

// ActionRunResult is the outcome of an executed governance command.
type ActionRunResult struct {
	ActionID   string                 `json:"action_id"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
	Status     string                 `json:"status"`
	Message    string                 `json:"message,omitempty"`
	Result     map[string]interface{} `json:"result,omitempty"`
}

// EventTypeStatusSource exposes per-event-type backlog for one outbox store.
type EventTypeStatusSource struct {
	Store  string
	Reader outboxport.EventTypeStatusReader
}
