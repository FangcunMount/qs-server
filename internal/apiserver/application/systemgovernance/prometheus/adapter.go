package prometheus

import (
	"context"
	"fmt"
	"sort"
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

// QuerySpec describes one bounded PromQL query and how to present the result.
type QuerySpec struct {
	Name   string
	Query  string
	Window string
	Unit   string
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

// Query executes one explicit PromQL query.
func (a *Adapter) Query(ctx context.Context, spec QuerySpec, evalAt time.Time) MetricResult {
	result := MetricResult{
		Name:   spec.Name,
		Window: spec.Window,
		Unit:   spec.Unit,
	}
	if a == nil || !a.enabled || a.client == nil {
		result.Reason = "prometheus not configured"
		return result
	}
	if strings.TrimSpace(spec.Query) == "" {
		result.Reason = "prometheus query is empty"
		return result
	}
	value, ok, err := a.client.QueryInstant(ctx, spec.Query, evalAt)
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

// CounterIncreaseQuery builds a near-window counter increase query.
func CounterIncreaseQuery(name, metric, window string, labels map[string]string) QuerySpec {
	return QuerySpec{
		Name:   name,
		Query:  fmt.Sprintf(`sum(increase(%s%s[%s]))`, metric, formatLabelSelector(labels), window),
		Window: window,
		Unit:   "count",
	}
}

// GaugeQuery wraps an instant gauge query.
func GaugeQuery(name, promQL, window, unit string) QuerySpec {
	return QuerySpec{
		Name:   name,
		Query:  promQL,
		Window: window,
		Unit:   unit,
	}
}

func formatLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for key, value := range labels {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, escapeLabelValue(value)))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"\n", `\n`,
		`"`, `\"`,
	)
	return replacer.Replace(value)
}
