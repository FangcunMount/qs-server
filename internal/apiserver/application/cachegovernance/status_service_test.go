package cachegovernance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

type stubCoordinator struct {
	snapshot WarmupStatusSnapshot
}

func (s stubCoordinator) WarmStartup(context.Context) error { return nil }

func (s stubCoordinator) HandleScalePublished(context.Context, string) error { return nil }

func (s stubCoordinator) HandleQuestionnairePublished(context.Context, string, string) error {
	return nil
}

func (s stubCoordinator) HandleStatisticsSync(context.Context, int64) error { return nil }

func (s stubCoordinator) HandleRepairComplete(context.Context, RepairCompleteRequest) error {
	return nil
}

func (s stubCoordinator) HandleManualWarmup(context.Context, ManualWarmupRequest) (*ManualWarmupResult, error) {
	return nil, nil
}

func (s stubCoordinator) Snapshot() WarmupStatusSnapshot { return s.snapshot }

type stubHotsetInspector struct {
	items []cachetarget.HotsetItem
	err   error
}

func (s stubHotsetInspector) TopWithScores(_ context.Context, _ redisplane.Family, _ cachetarget.WarmupKind, _ int64) ([]cachetarget.HotsetItem, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]cachetarget.HotsetItem(nil), s.items...), nil
}

func TestStatusServiceGetStatusFiltersComponentAndIncludesWarmupSnapshot(t *testing.T) {
	registry := cacheobservability.NewFamilyStatusRegistry("apiserver")
	registry.Update(cacheobservability.FamilyStatus{
		Family:      "query_result",
		Profile:     "query_cache",
		Namespace:   "prod:cache:query",
		AllowWarmup: true,
		Configured:  true,
		Available:   true,
		Mode:        cacheobservability.FamilyModeNamedProfile,
	})
	registry.Update(cacheobservability.FamilyStatus{
		Family:      "static_meta",
		Profile:     "static_cache",
		Namespace:   "prod:cache:static",
		AllowWarmup: true,
		Configured:  true,
		Available:   false,
		Degraded:    true,
		Mode:        cacheobservability.FamilyModeDegraded,
		LastError:   "redis unavailable",
	})
	registry.Update(cacheobservability.FamilyStatus{
		Component:   "worker",
		Family:      "lock_lease",
		Profile:     "lock_cache",
		Configured:  true,
		Available:   false,
		Degraded:    true,
		Mode:        cacheobservability.FamilyModeDegraded,
		LastError:   "redis unavailable",
		AllowWarmup: false,
	})

	expectedRun := WarmupRunSnapshot{
		Trigger:      "startup",
		StartedAt:    time.Unix(10, 0),
		FinishedAt:   time.Unix(11, 0),
		Result:       "ok",
		TargetCount:  3,
		OkCount:      3,
		ErrorCount:   0,
		SkippedCount: 0,
	}
	service := NewStatusService("apiserver", registry, nil, stubCoordinator{
		snapshot: WarmupStatusSnapshot{
			Enabled: true,
			Startup: WarmupStartupStatus{Static: true, Query: true},
			Hotset:  WarmupHotsetStatus{Enable: true, TopN: 20, MaxItemsPerKind: 200},
			LatestRuns: []WarmupRunSnapshot{
				expectedRun,
			},
		},
	})

	got, err := service.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if got.GeneratedAt.IsZero() {
		t.Fatal("GetStatus().GeneratedAt is zero")
	}
	if len(got.Families) != 2 {
		t.Fatalf("GetStatus().Families len = %d, want 2", len(got.Families))
	}
	if got.Families[0].Family != "query_result" || got.Families[1].Family != "static_meta" {
		t.Fatalf("GetStatus().Families order = [%q,%q], want [query_result,static_meta]", got.Families[0].Family, got.Families[1].Family)
	}
	if got.Families[0].Component != "apiserver" || got.Families[1].Component != "apiserver" {
		t.Fatalf("GetStatus().Families components = [%q,%q], want both apiserver", got.Families[0].Component, got.Families[1].Component)
	}
	if got.Summary.FamilyTotal != 2 {
		t.Fatalf("GetStatus().Summary.FamilyTotal = %d, want 2", got.Summary.FamilyTotal)
	}
	if got.Summary.AvailableCount != 1 {
		t.Fatalf("GetStatus().Summary.AvailableCount = %d, want 1", got.Summary.AvailableCount)
	}
	if got.Summary.DegradedCount != 1 {
		t.Fatalf("GetStatus().Summary.DegradedCount = %d, want 1", got.Summary.DegradedCount)
	}
	if got.Summary.UnavailableCount != 1 {
		t.Fatalf("GetStatus().Summary.UnavailableCount = %d, want 1", got.Summary.UnavailableCount)
	}
	if got.Summary.Ready {
		t.Fatal("GetStatus().Summary.Ready = true, want false")
	}
	if !got.Warmup.Enabled {
		t.Fatal("GetStatus().Warmup.Enabled = false, want true")
	}
	if !got.Warmup.Hotset.Enable {
		t.Fatal("GetStatus().Warmup.Hotset.Enable = false, want true")
	}
	if len(got.Warmup.LatestRuns) != 1 {
		t.Fatalf("GetStatus().Warmup.LatestRuns len = %d, want 1", len(got.Warmup.LatestRuns))
	}
	if got.Warmup.LatestRuns[0].Trigger != expectedRun.Trigger {
		t.Fatalf("GetStatus().Warmup.LatestRuns[0].Trigger = %q, want %q", got.Warmup.LatestRuns[0].Trigger, expectedRun.Trigger)
	}
}

func TestStatusServiceGetRuntimeUsesSharedSnapshotContract(t *testing.T) {
	registry := cacheobservability.NewFamilyStatusRegistry("apiserver")
	registry.Update(cacheobservability.FamilyStatus{
		Family:      "query_result",
		Profile:     "query_cache",
		Namespace:   "prod:cache:query",
		AllowWarmup: true,
		Configured:  true,
		Available:   true,
		Mode:        cacheobservability.FamilyModeNamedProfile,
	})
	registry.Update(cacheobservability.FamilyStatus{
		Family:      "static_meta",
		Profile:     "static_cache",
		Namespace:   "prod:cache:static",
		AllowWarmup: true,
		Configured:  true,
		Available:   false,
		Degraded:    true,
		Mode:        cacheobservability.FamilyModeDegraded,
		LastError:   "redis unavailable",
	})
	registry.Update(cacheobservability.FamilyStatus{
		Component: "worker",
		Family:    "lock_lease",
		Profile:   "lock_cache",
		Available: true,
		Mode:      cacheobservability.FamilyModeNamedProfile,
	})

	service := NewStatusService("apiserver", registry, nil, nil)

	got, err := service.GetRuntime(context.Background())
	if err != nil {
		t.Fatalf("GetRuntime() error = %v", err)
	}
	if got.Component != "apiserver" {
		t.Fatalf("GetRuntime().Component = %q, want apiserver", got.Component)
	}
	if got.Summary.FamilyTotal != 2 {
		t.Fatalf("GetRuntime().Summary.FamilyTotal = %d, want 2", got.Summary.FamilyTotal)
	}
	if got.Summary.AvailableCount != 1 {
		t.Fatalf("GetRuntime().Summary.AvailableCount = %d, want 1", got.Summary.AvailableCount)
	}
	if got.Summary.DegradedCount != 1 {
		t.Fatalf("GetRuntime().Summary.DegradedCount = %d, want 1", got.Summary.DegradedCount)
	}
	if got.Summary.UnavailableCount != 1 {
		t.Fatalf("GetRuntime().Summary.UnavailableCount = %d, want 1", got.Summary.UnavailableCount)
	}
	if got.Summary.Ready {
		t.Fatal("GetRuntime().Summary.Ready = true, want false")
	}
	if len(got.Families) != 2 {
		t.Fatalf("GetRuntime().Families len = %d, want 2", len(got.Families))
	}
	for _, family := range got.Families {
		if family.Component != "apiserver" {
			t.Fatalf("GetRuntime().Families component = %q, want apiserver", family.Component)
		}
	}
}

func TestStatusServiceGetHotsetReturnsItemsAndFamilyStatus(t *testing.T) {
	registry := cacheobservability.NewFamilyStatusRegistry("apiserver")
	registry.Update(cacheobservability.FamilyStatus{
		Family:      string(redisplane.FamilyQuery),
		Profile:     "query_cache",
		Namespace:   "prod:cache:query",
		AllowWarmup: true,
		Configured:  true,
		Available:   true,
		Mode:        cacheobservability.FamilyModeNamedProfile,
	})
	service := NewStatusService("apiserver", registry, stubHotsetInspector{
		items: []cachetarget.HotsetItem{
			{
				Target: cachetarget.NewQueryStatsSystemWarmupTarget(1),
				Score:  12,
			},
		},
	}, nil)

	got, err := service.GetHotset(context.Background(), cachetarget.WarmupKindQueryStatsSystem, 0)
	if err != nil {
		t.Fatalf("GetHotset() error = %v", err)
	}
	if got.Limit != 20 {
		t.Fatalf("GetHotset().Limit = %d, want 20", got.Limit)
	}
	if !got.Available {
		t.Fatal("GetHotset().Available = false, want true")
	}
	if got.Degraded {
		t.Fatal("GetHotset().Degraded = true, want false")
	}
	if got.Family != redisplane.FamilyQuery {
		t.Fatalf("GetHotset().Family = %q, want %q", got.Family, redisplane.FamilyQuery)
	}
	if len(got.Items) != 1 {
		t.Fatalf("GetHotset().Items len = %d, want 1", len(got.Items))
	}
	if got.Items[0].Target.Scope != "org:1" {
		t.Fatalf("GetHotset().Items[0].Target.Scope = %q, want org:1", got.Items[0].Target.Scope)
	}
}

func TestStatusServiceGetHotsetReturnsDegradedWhenInspectorFails(t *testing.T) {
	registry := cacheobservability.NewFamilyStatusRegistry("apiserver")
	registry.Update(cacheobservability.FamilyStatus{
		Family:      string(redisplane.FamilyStatic),
		Profile:     "static_cache",
		Namespace:   "prod:cache:static",
		AllowWarmup: true,
		Configured:  true,
		Available:   true,
		Mode:        cacheobservability.FamilyModeNamedProfile,
	})
	service := NewStatusService("apiserver", registry, stubHotsetInspector{
		err: errors.New("meta cache unavailable"),
	}, nil)

	got, err := service.GetHotset(context.Background(), cachetarget.WarmupKindStaticScale, 5)
	if err != nil {
		t.Fatalf("GetHotset() error = %v", err)
	}
	if got.Available {
		t.Fatal("GetHotset().Available = true, want false")
	}
	if !got.Degraded {
		t.Fatal("GetHotset().Degraded = false, want true")
	}
	if got.Message != "meta cache unavailable" {
		t.Fatalf("GetHotset().Message = %q, want %q", got.Message, "meta cache unavailable")
	}
}
