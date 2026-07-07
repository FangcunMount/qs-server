package prometheus

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
)

// Summary 报告Prometheus availability 用于 governance 视图。
type Summary struct {
	Available bool
	Reason    string
}

// MetricResult 是一个近窗口 指标观测。
type MetricResult struct {
	Name      string
	Window    string
	Value     *float64
	Unit      string
	Available bool
	Reason    string
}

// QuerySpec 描述一个有界 PromQL 查询 和 如何 存在 结果。
type QuerySpec struct {
	Name   string
	Query  string
	Window string
	Unit   string
}

// Adapter 加载近窗口 metrics via PromQL。
type Adapter struct {
	enabled bool
	client  *Client
}

// NewAdapter 构建metrics adapter 从 governance 选项。
func NewAdapter(opts *options.SystemGovernancePrometheusOptions) *Adapter {
	if opts == nil || !opts.Enabled || strings.TrimSpace(opts.BaseURL) == "" {
		return &Adapter{}
	}
	return &Adapter{
		enabled: true,
		client:  NewClient(opts.BaseURL, opts.Timeout),
	}
}

// Probe 检查Prometheus availability 不使用 failing caller。
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

// Query 执行一个显式 PromQL 查询。
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

// CounterIncreaseQuery 构建近窗口 counter increase 查询。
func CounterIncreaseQuery(name, metric, window string, labels map[string]string) QuerySpec {
	return QuerySpec{
		Name:   name,
		Query:  fmt.Sprintf(`sum(increase(%s%s[%s]))`, metric, formatLabelSelector(labels), window),
		Window: window,
		Unit:   "count",
	}
}

// GaugeQuery 包装即时仪表查询。
func GaugeQuery(name, promQL, window, unit string) QuerySpec {
	return QuerySpec{
		Name:   name,
		Query:  promQL,
		Window: window,
		Unit:   unit,
	}
}

// InstantGaugeQuery 构建sum() 即时仪表查询 使用 有界标签选择器。
func InstantGaugeQuery(name, metric, window, unit string, labels map[string]string) QuerySpec {
	return QuerySpec{
		Name:   name,
		Query:  fmt.Sprintf(`sum(%s%s)`, metric, formatLabelSelector(labels)),
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
