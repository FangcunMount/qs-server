package systemgovernance

import (
	"context"
	"sort"
	"time"

	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
)

// MetricsReader 加载近窗口 Prometheus 指标。
type MetricsReader interface {
	Query(ctx context.Context, spec govprom.QuerySpec, evalAt time.Time) govprom.MetricResult
}

func toMetricEvidence(item govprom.MetricResult) MetricEvidence {
	return MetricEvidence{
		Name:      item.Name,
		Window:    item.Window,
		Value:     item.Value,
		Unit:      item.Unit,
		Available: item.Available,
		Reason:    item.Reason,
	}
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// SortSignals orders 信号 按 severity then id。
func SortSignals(items []Signal) []Signal {
	sort.SliceStable(items, func(i, j int) bool {
		left := severityRank(items[i].Severity)
		right := severityRank(items[j].Severity)
		if left != right {
			return left > right
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityHealthy:
		return 1
	default:
		return 0
	}
}

// OverallSeverity 推导top severity 从 signal list。
func OverallSeverity(items []Signal) Severity {
	best := SeverityHealthy
	for _, item := range items {
		if severityRank(item.Severity) > severityRank(best) {
			best = item.Severity
		}
	}
	return best
}

// DomainSummaries 分组信号 按 领域。
func DomainSummaries(items []Signal) map[Domain]DomainSummary {
	result := map[Domain]DomainSummary{
		DomainEvents:     {Severity: SeverityHealthy},
		DomainCache:      {Severity: SeverityHealthy},
		DomainResilience: {Severity: SeverityHealthy},
	}
	for _, item := range items {
		summary := result[item.Domain]
		summary.SignalCount++
		if severityRank(item.Severity) > severityRank(summary.Severity) {
			summary.Severity = item.Severity
		}
		result[item.Domain] = summary
	}
	return result
}
