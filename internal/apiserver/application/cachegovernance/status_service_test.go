package cachegovernance

import (
	"context"
	"errors"
	"testing"
	"time"

	cacheinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
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

func (s stubCoordinator) Snapshot() WarmupStatusSnapshot { return s.snapshot }

type stubHotsetInspector struct {
	items []cacheinfra.HotsetItem
	err   error
}

func (s stubHotsetInspector) TopWithScores(_ context.Context, _ cacheinfra.CacheFamily, _ cacheinfra.WarmupKind, _ int64) ([]cacheinfra.HotsetItem, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]cacheinfra.HotsetItem(nil), s.items...), nil
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
	if len(got.Families) != 1 {
		t.Fatalf("GetStatus().Families len = %d, want 1", len(got.Families))
	}
	if got.Families[0].Component != "apiserver" {
		t.Fatalf("GetStatus().Families[0].Component = %q, want apiserver", got.Families[0].Component)
	}
	if got.Families[0].Family != "query_result" {
		t.Fatalf("GetStatus().Families[0].Family = %q, want query_result", got.Families[0].Family)
	}
	if !got.Warmup.Enabled {
		t.Fatal("GetStatus().Warmup.Enabled = false, want true")
	}
	if len(got.Warmup.LatestRuns) != 1 {
		t.Fatalf("GetStatus().Warmup.LatestRuns len = %d, want 1", len(got.Warmup.LatestRuns))
	}
	if got.Warmup.LatestRuns[0].Trigger != expectedRun.Trigger {
		t.Fatalf("GetStatus().Warmup.LatestRuns[0].Trigger = %q, want %q", got.Warmup.LatestRuns[0].Trigger, expectedRun.Trigger)
	}
}

func TestStatusServiceGetHotsetReturnsItemsAndFamilyStatus(t *testing.T) {
	registry := cacheobservability.NewFamilyStatusRegistry("apiserver")
	registry.Update(cacheobservability.FamilyStatus{
		Family:      string(cacheinfra.CacheFamilyQuery),
		Profile:     "query_cache",
		Namespace:   "prod:cache:query",
		AllowWarmup: true,
		Configured:  true,
		Available:   true,
		Mode:        cacheobservability.FamilyModeNamedProfile,
	})
	service := NewStatusService("apiserver", registry, stubHotsetInspector{
		items: []cacheinfra.HotsetItem{
			{
				Target: cacheinfra.NewQueryStatsSystemWarmupTarget(1),
				Score:  12,
			},
		},
	}, nil)

	got, err := service.GetHotset(context.Background(), cacheinfra.WarmupKindQueryStatsSystem, 0)
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
	if got.Family != cacheinfra.CacheFamilyQuery {
		t.Fatalf("GetHotset().Family = %q, want %q", got.Family, cacheinfra.CacheFamilyQuery)
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
		Family:      string(cacheinfra.CacheFamilyStatic),
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

	got, err := service.GetHotset(context.Background(), cacheinfra.WarmupKindStaticScale, 5)
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
