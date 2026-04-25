package resilienceplane

import (
	"context"
	"time"
)

type StatusService interface {
	GetStatus(context.Context) (*RuntimeSnapshot, error)
}

type StatusServiceFunc func(context.Context) (*RuntimeSnapshot, error)

func (f StatusServiceFunc) GetStatus(ctx context.Context) (*RuntimeSnapshot, error) {
	return f(ctx)
}

type RuntimeSnapshot struct {
	GeneratedAt          time.Time              `json:"generated_at"`
	Component            string                 `json:"component"`
	Summary              RuntimeSummary         `json:"summary"`
	RateLimits           []CapabilitySnapshot   `json:"rate_limits,omitempty"`
	Queues               []QueueSnapshot        `json:"queues,omitempty"`
	Backpressure         []BackpressureSnapshot `json:"backpressure,omitempty"`
	Locks                []CapabilitySnapshot   `json:"locks,omitempty"`
	Idempotency          []CapabilitySnapshot   `json:"idempotency,omitempty"`
	DuplicateSuppression []CapabilitySnapshot   `json:"duplicate_suppression,omitempty"`
}

type RuntimeSummary struct {
	Ready           bool `json:"ready"`
	CapabilityCount int  `json:"capability_count"`
	DegradedCount   int  `json:"degraded_count"`
}

type CapabilitySnapshot struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Strategy   string `json:"strategy"`
	Configured bool   `json:"configured"`
	Degraded   bool   `json:"degraded"`
	Reason     string `json:"reason,omitempty"`
}

type QueueSnapshot struct {
	GeneratedAt       time.Time      `json:"generated_at"`
	Component         string         `json:"component"`
	Name              string         `json:"name"`
	Strategy          string         `json:"strategy"`
	Depth             int            `json:"depth"`
	Capacity          int            `json:"capacity"`
	StatusTTLSeconds  int64          `json:"status_ttl_seconds"`
	StatusCounts      map[string]int `json:"status_counts"`
	LifecycleBoundary string         `json:"lifecycle_boundary"`
}

type BackpressureSnapshot struct {
	Component     string `json:"component"`
	Name          string `json:"name"`
	Dependency    string `json:"dependency"`
	Strategy      string `json:"strategy"`
	Enabled       bool   `json:"enabled"`
	MaxInflight   int    `json:"max_inflight"`
	InFlight      int    `json:"in_flight"`
	TimeoutMillis int64  `json:"timeout_millis"`
	Degraded      bool   `json:"degraded"`
	Reason        string `json:"reason,omitempty"`
}

func NewRuntimeSnapshot(component string, now time.Time) RuntimeSnapshot {
	if component == "" {
		component = "unknown"
	}
	if now.IsZero() {
		now = time.Now()
	}
	return RuntimeSnapshot{
		GeneratedAt: now,
		Component:   component,
		Summary: RuntimeSummary{
			Ready: true,
		},
	}
}

func FinalizeRuntimeSnapshot(snapshot RuntimeSnapshot) RuntimeSnapshot {
	count := 0
	degraded := 0
	accumulate := func(items []CapabilitySnapshot) {
		for _, item := range items {
			count++
			if item.Degraded {
				degraded++
			}
		}
	}
	accumulate(snapshot.RateLimits)
	accumulate(snapshot.Locks)
	accumulate(snapshot.Idempotency)
	accumulate(snapshot.DuplicateSuppression)
	for _, queue := range snapshot.Queues {
		count++
		if queue.Capacity <= 0 {
			degraded++
		}
	}
	for _, item := range snapshot.Backpressure {
		count++
		if item.Degraded {
			degraded++
		}
	}
	snapshot.Summary.CapabilityCount = count
	snapshot.Summary.DegradedCount = degraded
	snapshot.Summary.Ready = degraded == 0
	return snapshot
}
