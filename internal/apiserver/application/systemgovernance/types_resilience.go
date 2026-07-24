package systemgovernance

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

// ResilienceView 聚合 resilience 快照 across 组件。
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

// ComponentResilience 保存一个组件 resilience 载荷 使用 fetch 元数据。
type ComponentResilience struct {
	Available               bool                                   `json:"available"`
	Reason                  string                                 `json:"reason,omitempty"`
	Snapshot                *resilience.RuntimeSnapshot            `json:"snapshot,omitempty"`
	Instances               map[string]*resilience.RuntimeSnapshot `json:"instances,omitempty"`
	DiscoveredInstanceCount int                                    `json:"discovered_instance_count,omitempty"`
	AvailableInstanceCount  int                                    `json:"available_instance_count,omitempty"`
	Partial                 bool                                   `json:"partial,omitempty"`
	TargetErrors            map[string]string                      `json:"target_errors,omitempty"`
}

// ResilienceSummary 汇总压力保护健康度 across 组件。
type ResilienceSummary struct {
	ComponentCount             int     `json:"component_count"`
	UnavailableComponentCount  int     `json:"unavailable_component_count"`
	NotReadyComponentCount     int     `json:"not_ready_component_count"`
	InstanceCount              int     `json:"instance_count,omitempty"`
	NotReadyInstanceCount      int     `json:"not_ready_instance_count,omitempty"`
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

// ResilienceQueueRow 是面向 UI submit/worker queue pressure 行。
type ResilienceQueueRow struct {
	Component         string           `json:"component"`
	InstanceID        string           `json:"instance_id,omitempty"`
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

// ResilienceBackpressureRow 是面向 UI d负责tream backpressure 行。
type ResilienceBackpressureRow struct {
	Component      string           `json:"component"`
	InstanceID     string           `json:"instance_id,omitempty"`
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

// ResilienceCapabilityRow 是面向 UI non-queue resilience 能力 行。
type ResilienceCapabilityRow struct {
	Component  string   `json:"component"`
	InstanceID string   `json:"instance_id,omitempty"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Strategy   string   `json:"strategy"`
	Configured bool     `json:"configured"`
	Degraded   bool     `json:"degraded"`
	Severity   Severity `json:"severity"`
	Reason     string   `json:"reason,omitempty"`
}
