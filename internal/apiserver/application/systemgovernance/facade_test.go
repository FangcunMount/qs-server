package systemgovernance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cachegovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
)

func TestGetOverviewProbesMetricsOnce(t *testing.T) {
	metrics := &countingMetricsClient{}
	_, err := NewFacade(FacadeDeps{Metrics: metrics}).GetOverview(context.Background(), "5m")
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if metrics.probes != 1 {
		t.Fatalf("Probe calls = %d, want 1", metrics.probes)
	}
}

func TestGetOverviewDoesNotFetchCacheHotsets(t *testing.T) {
	governance := &cacheGovernanceForFacade{status: healthyCacheStatus("apiserver")}
	_, err := NewFacade(FacadeDeps{CacheGovernance: governance}).GetOverview(context.Background(), "5m")
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if governance.hotsetCalls != 0 {
		t.Fatalf("hotset calls = %d, want 0 for overview", governance.hotsetCalls)
	}
}

func TestGetCacheIncludesRemoteComponentDegradationAndHotsets(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "remote unavailable", http.StatusServiceUnavailable)
	}))
	defer remote.Close()
	governance := &cacheGovernanceForFacade{
		status: healthyCacheStatus("apiserver"),
		hotset: &cachegovernance.HotsetResponse{
			Family:    "query_result",
			Kind:      cachetarget.WarmupKindQueryStatsOverview,
			Limit:     5,
			Available: true,
			Items: []cachetarget.HotsetItem{
				{Target: cachetarget.NewQueryStatsOverviewWarmupTarget(9, "30d"), Score: 2},
			},
		},
	}
	view, err := NewFacade(FacadeDeps{
		CacheGovernance: governance,
		Components: govcomponent.NewAdapter(map[string]*options.GovernanceComponentOptions{
			"collection-server": {CacheURL: remote.URL},
		}),
	}).GetCache(context.Background(), "5m")
	if err != nil {
		t.Fatalf("GetCache() error = %v", err)
	}
	if view.Components["collection-server"].Available {
		t.Fatalf("remote component = %#v, want unavailable", view.Components["collection-server"])
	}
	if governance.hotsetCalls == 0 || len(view.Hotsets) == 0 {
		t.Fatalf("hotset calls = %d hotsets=%#v, want hotsets on cache detail", governance.hotsetCalls, view.Hotsets)
	}
	if len(view.Signals) == 0 {
		t.Fatalf("signals = %#v, want remote component warning signal", view.Signals)
	}
}

func TestGetCacheIncludesCanonicalCapabilityWorkloadRows(t *testing.T) {
	status := healthyCacheStatus("apiserver")
	status.EffectiveRegistry = &cachemodel.EffectiveRegistrySnapshot{
		Capabilities: []cachemodel.CapabilityPolicyView{
			{Capability: "statistics.query", Kind: "cache", Family: "query_result", MetricLabel: "stats_query"},
			{Capability: "report_status", Kind: "operational_state", Family: "ops_runtime", MetricLabel: "report_status"},
		},
	}
	view, err := NewFacade(FacadeDeps{
		CacheGovernance: &cacheGovernanceForFacade{status: status},
		Metrics:         &countingMetricsClient{},
	}).GetCache(context.Background(), "5m")
	if err != nil {
		t.Fatalf("GetCache() error = %v", err)
	}
	if len(view.CapabilityRows) != 1 {
		t.Fatalf("capability rows = %#v, want one workload cache row", view.CapabilityRows)
	}
	row := view.CapabilityRows[0]
	if row.Capability != "statistics.query" || row.MetricLabel != "stats_query" {
		t.Fatalf("capability row = %#v", row)
	}
	if row.Workload.HitRate == nil || row.Workload.ErrorCount == nil || row.Workload.GetLatencyP95 == nil {
		t.Fatalf("workload = %#v, want all metric evidences", row.Workload)
	}
}

func TestGetResilienceIncludesSummaryRowsAndRemoteDegradation(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "remote unavailable", http.StatusServiceUnavailable)
	}))
	defer remote.Close()

	view, err := NewFacade(FacadeDeps{
		LocalResilienceSnapshot: func() resilience.RuntimeSnapshot {
			snapshot := resilience.NewRuntimeSnapshot("apiserver", now)
			snapshot.Queues = []resilience.QueueSnapshot{{
				Component: "apiserver",
				Name:      "submit",
				Strategy:  "memory_channel",
				Depth:     75,
				Capacity:  100,
			}}
			return snapshot
		},
		Components: govcomponent.NewAdapter(map[string]*options.GovernanceComponentOptions{
			"collection-server": {ResilienceURL: remote.URL},
		}),
	}).GetResilience(context.Background(), "5m")
	if err != nil {
		t.Fatalf("GetResilience() error = %v", err)
	}
	if view.Summary.ComponentCount != 2 || view.Summary.UnavailableComponentCount != 1 || view.Summary.WarningQueueCount != 1 {
		t.Fatalf("summary = %#v, want local warning queue and remote unavailable component", view.Summary)
	}
	if len(view.QueueRows) != 1 || view.QueueRows[0].Name != "submit" {
		t.Fatalf("queue rows = %#v, want submit row", view.QueueRows)
	}
	if view.Components["collection-server"].Available {
		t.Fatalf("remote component = %#v, want unavailable", view.Components["collection-server"])
	}
}

type countingMetricsClient struct {
	probes int
}

func (c *countingMetricsClient) Probe(context.Context, time.Time) govprom.Summary {
	c.probes++
	return govprom.Summary{Available: true}
}

func (c *countingMetricsClient) Query(_ context.Context, spec govprom.QuerySpec, _ time.Time) govprom.MetricResult {
	return govprom.MetricResult{Name: spec.Name, Window: spec.Window, Unit: spec.Unit, Available: true}
}

type cacheGovernanceForFacade struct {
	status      *cachemodel.StatusSnapshot
	hotset      *cachegovernance.HotsetResponse
	hotsetCalls int
}

func (c *cacheGovernanceForFacade) TriggerStatisticsWarmup(context.Context, int64, string) {}

func (c *cacheGovernanceForFacade) HandleRepairComplete(context.Context, int64, cachegovernance.RepairCompleteRequest) error {
	return nil
}

func (c *cacheGovernanceForFacade) HandleManualWarmup(context.Context, int64, cachegovernance.ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error) {
	return nil, nil
}

func (c *cacheGovernanceForFacade) GetStatus(context.Context) (*cachemodel.StatusSnapshot, error) {
	return c.status, nil
}

func (c *cacheGovernanceForFacade) GetHotset(context.Context, string, string) (*cachegovernance.HotsetResponse, error) {
	c.hotsetCalls++
	return c.hotset, nil
}

func healthyCacheStatus(component string) *cachemodel.StatusSnapshot {
	return &cachemodel.StatusSnapshot{
		RuntimeSnapshot: cachemodel.RuntimeSnapshot{
			GeneratedAt: time.Now(),
			Component:   component,
			Summary:     cachemodel.RuntimeSummary{FamilyTotal: 1, AvailableCount: 1, Ready: true},
			Families: []cachemodel.FamilyStatus{
				{
					Component:   component,
					Family:      "query_result",
					Profile:     "query_cache",
					Namespace:   "qs:stats",
					AllowWarmup: true,
					Configured:  true,
					Available:   true,
					Mode:        observability.FamilyModeNamedProfile,
				},
			},
		},
	}
}
