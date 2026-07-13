package cachemodel

import "time"

// RuntimeSnapshot is the Redis-free cache runtime projection exposed to
// application consumers. Infrastructure converts redisruntime state into this
// contract at the governance boundary.
type RuntimeSnapshot struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Component   string         `json:"component"`
	Summary     RuntimeSummary `json:"summary"`
	Families    []FamilyStatus `json:"families"`
}

type RuntimeSummary struct {
	FamilyTotal      int  `json:"family_total"`
	AvailableCount   int  `json:"available_count"`
	DegradedCount    int  `json:"degraded_count"`
	UnavailableCount int  `json:"unavailable_count"`
	Ready            bool `json:"ready"`
}

type FamilyStatus struct {
	Component           string    `json:"component"`
	Family              string    `json:"family"`
	Profile             string    `json:"profile"`
	Namespace           string    `json:"namespace"`
	AllowWarmup         bool      `json:"allow_warmup"`
	Configured          bool      `json:"configured"`
	Available           bool      `json:"available"`
	Degraded            bool      `json:"degraded"`
	Mode                string    `json:"mode"`
	LastError           string    `json:"last_error,omitempty"`
	LastSuccessAt       time.Time `json:"last_success_at,omitempty"`
	LastFailureAt       time.Time `json:"last_failure_at,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type WarmupRunSnapshot struct {
	Trigger      string    `json:"trigger"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Result       string    `json:"result"`
	TargetCount  int       `json:"target_count"`
	OkCount      int       `json:"ok_count"`
	ErrorCount   int       `json:"error_count"`
	SkippedCount int       `json:"skipped_count"`
}

type WarmupStatusSnapshot struct {
	Enabled    bool                `json:"enabled"`
	Startup    WarmupStartupStatus `json:"startup"`
	Hotset     WarmupHotsetStatus  `json:"hotset"`
	LatestRuns []WarmupRunSnapshot `json:"latest_runs"`
}

type WarmupStartupStatus struct {
	Static bool `json:"static"`
	Query  bool `json:"query"`
}

type WarmupHotsetStatus struct {
	Enable          bool  `json:"enable"`
	TopN            int64 `json:"top_n"`
	MaxItemsPerKind int64 `json:"max_items_per_kind"`
}

type StatusSnapshot struct {
	RuntimeSnapshot
	Warmup WarmupStatusSnapshot `json:"warmup"`
}

type ManualWarmupItemStatus string

const (
	ManualWarmupItemStatusOK      ManualWarmupItemStatus = "ok"
	ManualWarmupItemStatusSkipped ManualWarmupItemStatus = "skipped"
	ManualWarmupItemStatusError   ManualWarmupItemStatus = "error"
)

type ManualWarmupSummary struct {
	TargetCount  int    `json:"target_count"`
	OkCount      int    `json:"ok_count"`
	SkippedCount int    `json:"skipped_count"`
	ErrorCount   int    `json:"error_count"`
	Result       string `json:"result"`
}

type ManualWarmupItemResult struct {
	Family  string                 `json:"family"`
	Kind    string                 `json:"kind"`
	Scope   string                 `json:"scope"`
	Status  ManualWarmupItemStatus `json:"status"`
	Message string                 `json:"message,omitempty"`
}

type ManualWarmupResult struct {
	Trigger    string                   `json:"trigger"`
	StartedAt  time.Time                `json:"started_at"`
	FinishedAt time.Time                `json:"finished_at"`
	Summary    ManualWarmupSummary      `json:"summary"`
	Items      []ManualWarmupItemResult `json:"items"`
}
