package systemgovernance

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// ResilienceView aggregates resilience snapshots across components.
type ResilienceView struct {
	GeneratedAt      time.Time                      `json:"generated_at"`
	Window           string                         `json:"window"`
	Metrics          MetricsSummary                 `json:"metrics"`
	Signals          []Signal                       `json:"signals"`
	Components       map[string]ComponentResilience `json:"components"`
	Summary          ResilienceSummary              `json:"summary"`
	QueueRows        []ResilienceQueueRow           `json:"queue_rows,omitempty"`
	BackpressureRows []ResilienceBackpressureRow    `json:"backpressure_rows,omitempty"`
	CapabilityRows   []ResilienceCapabilityRow      `json:"capability_rows,omitempty"`
}

// ComponentResilience holds one component resilience payload with fetch metadata.
type ComponentResilience struct {
	Available bool                             `json:"available"`
	Reason    string                           `json:"reason,omitempty"`
	Snapshot  *resilienceplane.RuntimeSnapshot `json:"snapshot,omitempty"`
}

// ResilienceSummary summarizes pressure-protection health across components.
type ResilienceSummary struct {
	ComponentCount             int     `json:"component_count"`
	UnavailableComponentCount  int     `json:"unavailable_component_count"`
	NotReadyComponentCount     int     `json:"not_ready_component_count"`
	QueueCount                 int     `json:"queue_count"`
	WarningQueueCount          int     `json:"warning_queue_count"`
	CriticalQueueCount         int     `json:"critical_queue_count"`
	MaxQueueUtilization        float64 `json:"max_queue_utilization"`
	BackpressureCount          int     `json:"backpressure_count"`
	WarningBackpressureCount   int     `json:"warning_backpressure_count"`
	CriticalBackpressureCount  int     `json:"critical_backpressure_count"`
	MaxBackpressureUtilization float64 `json:"max_backpressure_utilization"`
	DegradedCapabilityCount    int     `json:"degraded_capability_count"`
}

// ResilienceQueueRow is a UI-ready submit/worker queue pressure row.
type ResilienceQueueRow struct {
	Component         string           `json:"component"`
	Name              string           `json:"name"`
	Strategy          string           `json:"strategy"`
	Depth             int              `json:"depth"`
	Capacity          int              `json:"capacity"`
	Utilization       float64          `json:"utilization"`
	StatusCounts      map[string]int   `json:"status_counts,omitempty"`
	LifecycleBoundary string           `json:"lifecycle_boundary,omitempty"`
	Severity          Severity         `json:"severity"`
	Reason            string           `json:"reason,omitempty"`
	MetricEvidence    []MetricEvidence `json:"metric_evidence,omitempty"`
}

// ResilienceBackpressureRow is a UI-ready downstream backpressure row.
type ResilienceBackpressureRow struct {
	Component      string           `json:"component"`
	Name           string           `json:"name"`
	Dependency     string           `json:"dependency"`
	Strategy       string           `json:"strategy"`
	Enabled        bool             `json:"enabled"`
	InFlight       int              `json:"in_flight"`
	MaxInflight    int              `json:"max_inflight"`
	Utilization    float64          `json:"utilization"`
	TimeoutMillis  int64            `json:"timeout_millis"`
	Degraded       bool             `json:"degraded"`
	Severity       Severity         `json:"severity"`
	Reason         string           `json:"reason,omitempty"`
	MetricEvidence []MetricEvidence `json:"metric_evidence,omitempty"`
}

// ResilienceCapabilityRow is a UI-ready non-queue resilience capability row.
type ResilienceCapabilityRow struct {
	Component  string   `json:"component"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Strategy   string   `json:"strategy"`
	Configured bool     `json:"configured"`
	Degraded   bool     `json:"degraded"`
	Severity   Severity `json:"severity"`
	Reason     string   `json:"reason,omitempty"`
}
