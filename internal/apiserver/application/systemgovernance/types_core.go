package systemgovernance

import "time"

// Domain 标识治理关注域。
type Domain string

const (
	DomainEvents     Domain = "events"
	DomainCache      Domain = "cache"
	DomainResilience Domain = "resilience"
	DomainActions    Domain = "actions"
)

// Severity ranks diagnostic 信号。
type Severity string

const (
	SeverityHealthy  Severity = "healthy"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Signal 是有界 diagnostic 题目 用于 governance workbench。
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

// MetricEvidence 携带单一 近窗口 指标观测。
type MetricEvidence struct {
	Name      string   `json:"name"`
	Window    string   `json:"window"`
	Value     *float64 `json:"value,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Available bool     `json:"available"`
	Reason    string   `json:"reason,omitempty"`
}

// MetricsSummary 聚合 Prometheus availability 用于 视图。
type MetricsSummary struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// OverviewResponse 是top-等级 governance workbench 快照。
type OverviewResponse struct {
	GeneratedAt     time.Time                `json:"generated_at"`
	Window          string                   `json:"window"`
	OverallSeverity Severity                 `json:"overall_severity"`
	Metrics         MetricsSummary           `json:"metrics"`
	Signals         []Signal                 `json:"signals"`
	Domains         map[Domain]DomainSummary `json:"domains"`
}

// DomainSummary 汇总一个领域 in 概览。
type DomainSummary struct {
	Severity    Severity `json:"severity"`
	SignalCount int      `json:"signal_count"`
}
