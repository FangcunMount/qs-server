package systemgovernance

import "time"

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
