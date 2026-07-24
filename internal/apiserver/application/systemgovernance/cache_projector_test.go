package systemgovernance

import (
	"context"
	"strings"
	"testing"
	"time"

	cachegovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

func TestCacheWarmupProjectionBuildsRowsKindsHotsetsAndScopedMetrics(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	metrics := &recordingMetricsReader{}
	runtime := observability.RuntimeSnapshot{
		GeneratedAt: now,
		Component:   "apiserver",
		Summary:     observability.RuntimeSummary{FamilyTotal: 1, DegradedCount: 1, Ready: false},
		Families: []observability.FamilyStatus{
			{
				Component:           "apiserver",
				Family:              "query_result",
				Profile:             "query_cache",
				Namespace:           "qs:stats",
				AllowWarmup:         true,
				Configured:          true,
				Available:           true,
				Degraded:            true,
				Mode:                "degraded",
				ConsecutiveFailures: 2,
			},
		},
	}
	hotset := CacheHotsetViewFromResponse(cachetarget.WarmupKindQueryStatsOverview, &cachegovernance.HotsetResponse{
		Family:    "query_result",
		Kind:      cachetarget.WarmupKindQueryStatsOverview,
		Limit:     5,
		Available: true,
		Items: []cachetarget.HotsetItem{
			{Target: cachetarget.NewQueryStatsOverviewWarmupTarget(7, "30d"), Score: 3},
		},
	}, nil)

	projection := NewCacheWarmupEvaluator(metrics).Evaluate(context.Background(), map[string]ComponentCache{
		"apiserver":         {Available: true, Snapshot: &runtime},
		"collection-server": {Available: false, Reason: "connection refused"},
	}, []CacheHotsetView{hotset}, "5m", now)

	if len(projection.FamilyRows) != 1 || projection.FamilyRows[0].Severity != SeverityWarning {
		t.Fatalf("family rows = %#v, want one warning row", projection.FamilyRows)
	}
	if len(projection.WarmupKinds) != 4 {
		t.Fatalf("warmup kinds len = %d, want 4", len(projection.WarmupKinds))
	}
	for _, descriptor := range projection.WarmupKinds {
		if _, ok := cachetarget.ParseWarmupKind(string(descriptor.Kind)); !ok {
			t.Fatalf("warmup kind descriptor uses unsupported kind: %#v", descriptor)
		}
	}
	if len(projection.Hotsets) != 1 || len(projection.Hotsets[0].Items) != 1 {
		t.Fatalf("hotsets = %#v, want one recommended target", projection.Hotsets)
	}
	var sawFamilyMetric bool
	for _, spec := range metrics.specs {
		if strings.Contains(spec.Query, `component="apiserver"`) && strings.Contains(spec.Query, `family="query_result"`) {
			sawFamilyMetric = true
			break
		}
	}
	if !sawFamilyMetric {
		t.Fatalf("metric specs = %#v, want component/family scoped cache metric", metrics.specs)
	}
}

func TestCacheWarmupProjectionMarksHotsetDegraded(t *testing.T) {
	projection := NewCacheWarmupEvaluator(nil).Evaluate(context.Background(), nil, []CacheHotsetView{
		CacheHotsetViewFromResponse(cachetarget.WarmupKindStaticScale, nil, nil),
	}, "5m", time.Now())
	if len(projection.Signals) != 1 || projection.Signals[0].Status != "hotset_degraded" {
		t.Fatalf("signals = %#v, want hotset degraded signal", projection.Signals)
	}
}

func TestCacheCapabilityRowsProjectCanonicalCapabilitiesAndLegacyMetricLabels(t *testing.T) {
	metrics := &recordingMetricsReader{}
	rows := NewCacheWarmupEvaluator(metrics).CapabilityRows(context.Background(), &cachemodel.EffectiveRegistrySnapshot{
		Capabilities: []cachemodel.CapabilityPolicyView{
			{
				Capability: "statistics.query", Kind: "cache", Family: "query_result", MetricLabel: "stats_query",
			},
			{
				Capability: "report_status", Kind: "operational_state", Family: "ops_runtime", MetricLabel: "report_status",
			},
		},
	}, "5m", time.Now())

	if len(rows) != 1 {
		t.Fatalf("capability rows = %#v, want one workload cache row", rows)
	}
	row := rows[0]
	if row.Capability != "statistics.query" || row.MetricLabel != "stats_query" || row.Family != "query_result" {
		t.Fatalf("capability row = %#v, want canonical capability and legacy metric labels", row)
	}
	if row.Workload.HitRate == nil || row.Workload.ErrorCount == nil || row.Workload.GetLatencyP95 == nil {
		t.Fatalf("workload = %#v, want three metric evidences", row.Workload)
	}
	if len(metrics.specs) != 3 {
		t.Fatalf("metric specs = %#v, want three workload queries", metrics.specs)
	}
	for _, spec := range metrics.specs {
		if !strings.Contains(spec.Query, `family="query_result"`) || !strings.Contains(spec.Query, `policy="stats_query"`) {
			t.Fatalf("query = %q, want legacy family/policy labels", spec.Query)
		}
	}
}

func TestCacheProjectionKeepsDNSInstancesSeparate(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	newSnapshot := func(instanceID string) *observability.RuntimeSnapshot {
		return &observability.RuntimeSnapshot{
			GeneratedAt: now,
			Component:   "collection-server",
			InstanceID:  instanceID,
			Summary:     observability.RuntimeSummary{Ready: true, FamilyTotal: 1, AvailableCount: 1},
			Families: []observability.FamilyStatus{{
				Component: "collection-server", Family: "ops_runtime",
				Profile: "ops_runtime", Available: true,
			}},
		}
	}

	projection := NewCacheWarmupEvaluator(nil).Evaluate(context.Background(), map[string]ComponentCache{
		"collection-server": {
			Available: true,
			Instances: map[string]*observability.RuntimeSnapshot{
				"collection-b": newSnapshot("collection-b"),
				"collection-a": newSnapshot("collection-a"),
			},
		},
	}, nil, "5m", now)

	if len(projection.FamilyRows) != 2 ||
		projection.FamilyRows[0].InstanceID != "collection-a" ||
		projection.FamilyRows[1].InstanceID != "collection-b" {
		t.Fatalf("family rows = %#v, want two stable instance rows", projection.FamilyRows)
	}
	if projection.FamilyRows[0].Component != "collection-server" {
		t.Fatalf("row = %#v, want component preserved", projection.FamilyRows[0])
	}
}
