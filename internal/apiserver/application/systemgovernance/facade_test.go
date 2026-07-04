package systemgovernance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
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
		hotset: &statisticsApp.GovernanceHotsetResponse{
			Family:    "query_result",
			Kind:      cachetarget.WarmupKindQueryStatsSystem,
			Limit:     5,
			Available: true,
			Items: []cachetarget.HotsetItem{
				{Target: cachetarget.NewQueryStatsSystemWarmupTarget(9), Score: 2},
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

func TestGetResilienceIncludesSummaryRowsAndRemoteDegradation(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "remote unavailable", http.StatusServiceUnavailable)
	}))
	defer remote.Close()

	view, err := NewFacade(FacadeDeps{
		LocalResilienceSnapshot: func() resilienceplane.RuntimeSnapshot {
			snapshot := resilienceplane.NewRuntimeSnapshot("apiserver", now)
			snapshot.Queues = []resilienceplane.QueueSnapshot{{
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
	status      *cachegov.StatusSnapshot
	hotset      *statisticsApp.GovernanceHotsetResponse
	hotsetCalls int
}

func (c *cacheGovernanceForFacade) TriggerStatisticsWarmup(context.Context, int64, string) {}

func (c *cacheGovernanceForFacade) HandleRepairComplete(context.Context, int64, statisticsApp.RepairCompleteRequest) error {
	return nil
}

func (c *cacheGovernanceForFacade) HandleManualWarmup(context.Context, int64, statisticsApp.ManualWarmupRequest) (*cachegov.ManualWarmupResult, error) {
	return nil, nil
}

func (c *cacheGovernanceForFacade) GetStatus(context.Context) (*cachegov.StatusSnapshot, error) {
	return c.status, nil
}

func (c *cacheGovernanceForFacade) GetHotset(context.Context, string, string) (*statisticsApp.GovernanceHotsetResponse, error) {
	c.hotsetCalls++
	return c.hotset, nil
}

func healthyCacheStatus(component string) *cachegov.StatusSnapshot {
	return &cachegov.StatusSnapshot{
		RuntimeSnapshot: observability.RuntimeSnapshot{
			GeneratedAt: time.Now(),
			Component:   component,
			Summary:     observability.RuntimeSummary{FamilyTotal: 1, AvailableCount: 1, Ready: true},
			Families: []observability.FamilyStatus{
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
