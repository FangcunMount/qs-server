package prometheus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
)

// Summary reports Prometheus availability for governance views.
type Summary struct {
	Available bool
	Reason    string
}

// MetricResult is one near-window metric observation.
type MetricResult struct {
	Name      string
	Window    string
	Value     *float64
	Unit      string
	Available bool
	Reason    string
}

// Adapter loads near-window metrics via PromQL.
type Adapter struct {
	enabled bool
	client  *Client
}

// NewAdapter builds a metrics adapter from governance options.
func NewAdapter(opts *options.SystemGovernancePrometheusOptions) *Adapter {
	if opts == nil || !opts.Enabled || strings.TrimSpace(opts.BaseURL) == "" {
		return &Adapter{}
	}
	return &Adapter{
		enabled: true,
		client:  NewClient(opts.BaseURL, opts.Timeout),
	}
}

// Probe checks Prometheus availability without failing the caller.
func (a *Adapter) Probe(ctx context.Context, evalAt time.Time) Summary {
	if a == nil || !a.enabled || a.client == nil {
		return Summary{
			Available: false,
			Reason:    "prometheus not configured",
		}
	}
	_, ok, err := a.client.QueryInstant(ctx, "up", evalAt)
	if err != nil {
		return Summary{Available: false, Reason: err.Error()}
	}
	if !ok {
		return Summary{Available: false, Reason: "prometheus returned empty result for up"}
	}
	return Summary{Available: true}
}

// QueryIncrease returns one counter increase metric.
func (a *Adapter) QueryIncrease(ctx context.Context, metricName, window string, labels map[string]string, evalAt time.Time) MetricResult {
	result := MetricResult{
		Name:   metricName,
		Window: window,
		Unit:   "count",
	}
	if a == nil || !a.enabled || a.client == nil {
		result.Reason = "prometheus not configured"
		return result
	}
	labelExpr := formatLabelSelector(labels)
	query := fmt.Sprintf(`sum(increase(qs_resilience_decision_total%s[%s]))`, labelExpr, window)
	value, ok, err := a.client.QueryInstant(ctx, query, evalAt)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	result.Available = true
	if ok {
		result.Value = &value
	}
	return result
}

// QueryGauge returns one instant gauge metric.
func (a *Adapter) QueryGauge(ctx context.Context, metricName, promQL, window, unit string, evalAt time.Time) MetricResult {
	result := MetricResult{
		Name:   metricName,
		Window: window,
		Unit:   unit,
	}
	if a == nil || !a.enabled || a.client == nil {
		result.Reason = "prometheus not configured"
		return result
	}
	value, ok, err := a.client.QueryInstant(ctx, promQL, evalAt)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	result.Available = true
	if ok {
		result.Value = &value
	}
	return result
}

func formatLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for key, value := range labels {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, value))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
