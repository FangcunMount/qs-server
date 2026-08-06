package main

import (
	"encoding/json"
	"time"
)

const reportSchemaVersion = "qs-perf-report/v1"

type VerdictStatus string

const (
	VerdictPass       VerdictStatus = "PASS"
	VerdictFail       VerdictStatus = "FAIL"
	VerdictIncomplete VerdictStatus = "INCOMPLETE"
	VerdictError      VerdictStatus = "ERROR"
)

type Verdict struct {
	Status  VerdictStatus `json:"status"`
	Reasons []string      `json:"reasons"`
}

type RunMetadata struct {
	ID          string    `json:"id"`
	Plan        string    `json:"plan"`
	GitSHA      string    `json:"git_sha"`
	GitDirty    bool      `json:"git_dirty"`
	Environment string    `json:"environment"`
	K6Version   string    `json:"k6_version"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	ConfigFile  string    `json:"config_file"`
}

type Measurement struct {
	Value   *float64 `json:"value"`
	Unit    string   `json:"unit"`
	Samples int64    `json:"samples,omitempty"`
	Source  string   `json:"source"`
	Note    string   `json:"note,omitempty"`
}

type BusinessQPS struct {
	Target           Measurement `json:"target"`
	Actual           Measurement `json:"actual"`
	TargetAttainment Measurement `json:"target_attainment"`
	Dropped          Measurement `json:"dropped_iterations"`
}

type Throughput struct {
	BusinessQPS          BusinessQPS            `json:"business_qps"`
	HTTPRPS              Measurement            `json:"http_rps"`
	WSSessionsPerSecond  Measurement            `json:"ws_sessions_per_second"`
	AcceptedTPS          Measurement            `json:"accepted_tps"`
	AcceptedTPSByModel   map[string]Measurement `json:"accepted_tps_by_model"`
	CompletedTPS         Measurement            `json:"completed_tps"`
	CompletedTPSByModel  map[string]Measurement `json:"completed_tps_by_model"`
	FinalCompletionRate  Measurement            `json:"final_completion_rate"`
	RequestAmplification Measurement            `json:"request_amplification"`
	PollingAmplification Measurement            `json:"polling_amplification"`
}

type LatencyMetric struct {
	Operation string      `json:"operation"`
	Samples   int64       `json:"samples"`
	P50       Measurement `json:"p50"`
	P95       Measurement `json:"p95"`
	P99       Measurement `json:"p99"`
	Max       Measurement `json:"max"`
	Average   Measurement `json:"average"`
}

type CorrectnessMetric struct {
	Operation     string      `json:"operation"`
	Attempts      int64       `json:"attempts"`
	SuccessCount  *int64      `json:"success_count"`
	ErrorCount    *int64      `json:"error_count"`
	TimeoutCount  *int64      `json:"timeout_count"`
	SuccessRate   Measurement `json:"success_rate"`
	ErrorRate     Measurement `json:"error_rate"`
	TimeoutRate   Measurement `json:"timeout_rate"`
	FinalFailRate Measurement `json:"final_failure_rate"`
	Idempotency   Measurement `json:"idempotency_success_rate"`
}

type RetryMetric struct {
	Layer           string      `json:"layer"`
	InitialAttempts int64       `json:"initial_attempts"`
	RetryAttempts   int64       `json:"retry_attempts"`
	RetryRate       Measurement `json:"retry_rate"`
	Outcome         string      `json:"outcome,omitempty"`
}

type QueueWaitMetric struct {
	Layer string      `json:"layer"`
	Wait  Measurement `json:"wait"`
}

type EvidenceCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Source  string `json:"source"`
	Message string `json:"message,omitempty"`
}

type PhaseEvidence struct {
	Complete                   bool               `json:"complete"`
	TrafficIsolated            *bool              `json:"traffic_isolated"`
	Checks                     []EvidenceCheck    `json:"checks"`
	CompletedCountDelta        *float64           `json:"completed_count_delta"`
	FailedCountDelta           *float64           `json:"failed_count_delta"`
	CompletedCountDeltaByModel map[string]float64 `json:"completed_count_delta_by_model"`
	Retry                      []RetryMetric      `json:"retry"`
	QueueWait                  []QueueWaitMetric  `json:"queue_wait"`
	OutboxBacklog              *float64           `json:"outbox_backlog"`
	OutboxOldestAge            *float64           `json:"outbox_oldest_age_seconds"`
	NSQDepth                   *float64           `json:"nsq_depth"`
}

type ThresholdResult struct {
	Metric     string `json:"metric"`
	Expression string `json:"expression"`
	Passed     bool   `json:"passed"`
}

type PhaseSummary struct {
	ID             string              `json:"id"`
	Profile        string              `json:"profile"`
	TargetQPS      int                 `json:"target_qps"`
	Duration       string              `json:"duration"`
	ActualDuration Measurement         `json:"actual_duration"`
	ThresholdTier  string              `json:"threshold_tier"`
	StartedAt      time.Time           `json:"started_at"`
	FinishedAt     time.Time           `json:"finished_at"`
	Verdict        Verdict             `json:"verdict"`
	Thresholds     []ThresholdResult   `json:"thresholds"`
	Throughput     Throughput          `json:"throughput"`
	Latency        []LatencyMetric     `json:"latency"`
	Correctness    []CorrectnessMetric `json:"correctness"`
	Retry          []RetryMetric       `json:"retry"`
	QueueWait      []QueueWaitMetric   `json:"queue_wait"`
	Evidence       PhaseEvidence       `json:"evidence"`
}

type RecoverySummary struct {
	ID         string        `json:"id"`
	Verdict    Verdict       `json:"verdict"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Attempts   int           `json:"attempts"`
	Evidence   PhaseEvidence `json:"evidence"`
}

type RunSummary struct {
	SchemaVersion string                         `json:"schema_version"`
	Run           RunMetadata                    `json:"run"`
	Verdict       Verdict                        `json:"verdict"`
	Phases        []PhaseSummary                 `json:"phases"`
	Throughput    map[string]Throughput          `json:"throughput"`
	Latency       map[string][]LatencyMetric     `json:"latency"`
	Correctness   map[string][]CorrectnessMetric `json:"correctness"`
	Retry         map[string][]RetryMetric       `json:"retry"`
	Recovery      []RecoverySummary              `json:"recovery"`
}

type rawSummary struct {
	Metrics   map[string]map[string]any `json:"metrics"`
	RootGroup json.RawMessage           `json:"root_group,omitempty"`
	SetupData json.RawMessage           `json:"setup_data,omitempty"`
}

func floatPtr(value float64) *float64 { return &value }

func int64Ptr(value int64) *int64 { return &value }

func measured(value *float64, unit, source string) Measurement {
	return Measurement{Value: value, Unit: unit, Source: source}
}

func naMeasurement(unit, source, note string) Measurement {
	return Measurement{Value: nil, Unit: unit, Source: source, Note: note}
}
