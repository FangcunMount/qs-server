package cachegovernance

import (
	"context"
	"fmt"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
)

func TestCoordinatorHandleManualWarmupReturnsStructuredResultAndRecordsRun(t *testing.T) {
	var scaleCalls []string
	var overviewCalls []string

	coord := NewCoordinator(Config{Enable: true}, Dependencies{
		Runtime: NewFamilyRuntime(map[cachemodel.Family]bool{
			cachemodel.FamilyStatic: true,
			cachemodel.FamilyQuery:  true,
		}),
		WarmScale: func(_ context.Context, code string) error {
			scaleCalls = append(scaleCalls, code)
			return nil
		},
		WarmStatsOverview: func(_ context.Context, orgID int64, preset string) error {
			overviewCalls = append(overviewCalls, fmt.Sprintf("%d:%s", orgID, preset))
			return nil
		},
	})

	result, err := coord.HandleManualWarmup(context.Background(), cachetarget.ManualWarmupRequest{
		Targets: []cachetarget.ManualWarmupTarget{
			{Kind: "static.scale", Scope: "scale:S-001"},
			{Kind: "query.stats_overview", Scope: "org:2:preset:30d"},
		},
	})
	if err != nil {
		t.Fatalf("HandleManualWarmup() error = %v", err)
	}
	if result.Trigger != manualWarmupTrigger {
		t.Fatalf("trigger = %q, want %q", result.Trigger, manualWarmupTrigger)
	}
	if result.Summary.TargetCount != 2 {
		t.Fatalf("target_count = %d, want 2", result.Summary.TargetCount)
	}
	if result.Summary.OkCount != 2 {
		t.Fatalf("ok_count = %d, want 2", result.Summary.OkCount)
	}
	if result.Summary.Result != "ok" {
		t.Fatalf("result = %q, want ok", result.Summary.Result)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Status != cachemodel.ManualWarmupItemStatusOK {
			t.Fatalf("unexpected item statuses: %+v", result.Items)
		}
	}
	if len(scaleCalls) != 1 || scaleCalls[0] != "s-001" {
		t.Fatalf("scale warmup calls = %v, want [s-001]", scaleCalls)
	}
	if len(overviewCalls) != 1 || overviewCalls[0] != "2:30d" {
		t.Fatalf("overview warmup calls = %v, want [2:30d]", overviewCalls)
	}

	snapshot := coord.Snapshot()
	if len(snapshot.LatestRuns) != 1 {
		t.Fatalf("latest runs len = %d, want 1", len(snapshot.LatestRuns))
	}
	if snapshot.LatestRuns[0].Trigger != manualWarmupTrigger {
		t.Fatalf("latest run trigger = %q, want %q", snapshot.LatestRuns[0].Trigger, manualWarmupTrigger)
	}
}

func TestCoordinatorHandleStatisticsSyncWarmsOverviewPresets(t *testing.T) {
	var overviewCalls []string

	coord := NewCoordinator(Config{Enable: true}, Dependencies{
		Runtime: NewFamilyRuntime(map[cachemodel.Family]bool{
			cachemodel.FamilyQuery: true,
		}),
		WarmStatsOverview: func(_ context.Context, orgID int64, preset string) error {
			overviewCalls = append(overviewCalls, fmt.Sprintf("%d:%s", orgID, preset))
			return nil
		},
	})

	if err := coord.HandleStatisticsSync(context.Background(), 9); err != nil {
		t.Fatalf("HandleStatisticsSync() error = %v", err)
	}

	wantOverview := map[string]bool{
		"9:today": false,
		"9:7d":    false,
		"9:30d":   false,
	}
	for _, call := range overviewCalls {
		if _, ok := wantOverview[call]; ok {
			wantOverview[call] = true
		}
	}
	for call, seen := range wantOverview {
		if !seen {
			t.Fatalf("overview warmup calls = %v, missing %s", overviewCalls, call)
		}
	}
	if len(overviewCalls) != len(wantOverview) {
		t.Fatalf("overview warmup calls = %v, want only %v", overviewCalls, wantOverview)
	}

	snapshot := coord.Snapshot()
	if len(snapshot.LatestRuns) != 1 {
		t.Fatalf("latest runs len = %d, want 1", len(snapshot.LatestRuns))
	}
	run := snapshot.LatestRuns[0]
	if run.Trigger != "statistics_sync" || run.TargetCount != 3 || run.OkCount != 3 {
		t.Fatalf("unexpected statistics_sync run: %+v", run)
	}
}

func TestCoordinatorWarmStartupUsesStatisticsSeedsWhenWarmOnStartup(t *testing.T) {
	var overviewCalls []string

	coord := NewCoordinator(Config{Enable: true, StartupStatic: false, StartupQuery: false}, Dependencies{
		Runtime: NewFamilyRuntime(map[cachemodel.Family]bool{
			cachemodel.FamilyQuery: true,
		}),
		StatisticsSeeds: &StatisticsWarmupConfig{
			OrgIDs:          []int64{1},
			OverviewPresets: []string{"7d"},
			WarmOnStartup:   true,
		},
		WarmStatsOverview: func(_ context.Context, orgID int64, preset string) error {
			overviewCalls = append(overviewCalls, fmt.Sprintf("%d:%s", orgID, preset))
			return nil
		},
	})

	if err := coord.WarmStartup(context.Background()); err != nil {
		t.Fatalf("WarmStartup() error = %v", err)
	}
	if len(overviewCalls) != 1 || overviewCalls[0] != "1:7d" {
		t.Fatalf("overview warmup calls = %v, want [1:7d]", overviewCalls)
	}
}

func TestCoordinatorHandleRepairCompleteStillUsesStructuredExecutor(t *testing.T) {
	var overviewCalls []string

	coord := NewCoordinator(Config{Enable: true}, Dependencies{
		Runtime: NewFamilyRuntime(map[cachemodel.Family]bool{
			cachemodel.FamilyQuery: true,
		}),
		WarmStatsOverview: func(_ context.Context, orgID int64, preset string) error {
			overviewCalls = append(overviewCalls, fmt.Sprintf("%d:%s", orgID, preset))
			return nil
		},
	})

	if err := coord.HandleRepairComplete(context.Background(), cachetarget.RepairCompleteRequest{
		RepairKind: "statistics_backfill",
		OrgIDs:     []int64{1},
	}); err != nil {
		t.Fatalf("HandleRepairComplete() error = %v", err)
	}

	if len(overviewCalls) != 3 {
		t.Fatalf("overview warmup calls = %v, want three presets", overviewCalls)
	}

	snapshot := coord.Snapshot()
	if len(snapshot.LatestRuns) != 1 {
		t.Fatalf("latest runs len = %d, want 1", len(snapshot.LatestRuns))
	}
	if snapshot.LatestRuns[0].Trigger != "repair" {
		t.Fatalf("latest run trigger = %q, want repair", snapshot.LatestRuns[0].Trigger)
	}
}
