package cachegovernance

import (
	"context"
	"fmt"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachemodel"
)

func TestCoordinatorHandleManualWarmupReturnsStructuredResultAndRecordsRun(t *testing.T) {
	var scaleCalls []string
	var overviewCalls []string
	var systemCalls []int64

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
		WarmStatsSystem: func(_ context.Context, orgID int64) error {
			systemCalls = append(systemCalls, orgID)
			return nil
		},
	})

	result, err := coord.HandleManualWarmup(context.Background(), ManualWarmupRequest{
		Targets: []ManualWarmupTarget{
			{Kind: "static.scale", Scope: "scale:S-001"},
			{Kind: "query.stats_overview", Scope: "org:2:preset:30d"},
			{Kind: "query.stats_system", Scope: "org:1"},
		},
	})
	if err != nil {
		t.Fatalf("HandleManualWarmup() error = %v", err)
	}
	if result.Trigger != manualWarmupTrigger {
		t.Fatalf("trigger = %q, want %q", result.Trigger, manualWarmupTrigger)
	}
	if result.Summary.TargetCount != 3 {
		t.Fatalf("target_count = %d, want 3", result.Summary.TargetCount)
	}
	if result.Summary.OkCount != 3 {
		t.Fatalf("ok_count = %d, want 3", result.Summary.OkCount)
	}
	if result.Summary.Result != "ok" {
		t.Fatalf("result = %q, want ok", result.Summary.Result)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items len = %d, want 3", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Status != ManualWarmupItemStatusOK {
			t.Fatalf("unexpected item statuses: %+v", result.Items)
		}
	}
	if len(scaleCalls) != 1 || scaleCalls[0] != "s-001" {
		t.Fatalf("scale warmup calls = %v, want [s-001]", scaleCalls)
	}
	if len(overviewCalls) != 1 || overviewCalls[0] != "2:30d" {
		t.Fatalf("overview warmup calls = %v, want [2:30d]", overviewCalls)
	}
	if len(systemCalls) != 1 || systemCalls[0] != 1 {
		t.Fatalf("system warmup calls = %v, want [1]", systemCalls)
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
	var systemCalls []int64

	coord := NewCoordinator(Config{Enable: true}, Dependencies{
		Runtime: NewFamilyRuntime(map[cachemodel.Family]bool{
			cachemodel.FamilyQuery: true,
		}),
		WarmStatsOverview: func(_ context.Context, orgID int64, preset string) error {
			overviewCalls = append(overviewCalls, fmt.Sprintf("%d:%s", orgID, preset))
			return nil
		},
		WarmStatsSystem: func(_ context.Context, orgID int64) error {
			systemCalls = append(systemCalls, orgID)
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
	if len(systemCalls) != 1 || systemCalls[0] != 9 {
		t.Fatalf("system warmup calls = %v, want [9]", systemCalls)
	}

	snapshot := coord.Snapshot()
	if len(snapshot.LatestRuns) != 1 {
		t.Fatalf("latest runs len = %d, want 1", len(snapshot.LatestRuns))
	}
	run := snapshot.LatestRuns[0]
	if run.Trigger != "statistics_sync" || run.TargetCount != 4 || run.OkCount != 4 {
		t.Fatalf("unexpected statistics_sync run: %+v", run)
	}
}

func TestCoordinatorHandleRepairCompleteStillUsesStructuredExecutor(t *testing.T) {
	var questionnaireCalls []string

	coord := NewCoordinator(Config{Enable: true}, Dependencies{
		Runtime: NewFamilyRuntime(map[cachemodel.Family]bool{
			cachemodel.FamilyQuery: true,
		}),
		WarmStatsQuestionnaire: func(_ context.Context, _ int64, code string) error {
			questionnaireCalls = append(questionnaireCalls, code)
			return nil
		},
	})

	if err := coord.HandleRepairComplete(context.Background(), RepairCompleteRequest{
		RepairKind:         "statistics_backfill",
		OrgIDs:             []int64{1},
		QuestionnaireCodes: []string{"Q-001"},
	}); err != nil {
		t.Fatalf("HandleRepairComplete() error = %v", err)
	}

	if len(questionnaireCalls) != 1 || questionnaireCalls[0] != "q-001" {
		t.Fatalf("questionnaire warmup calls = %v, want [q-001]", questionnaireCalls)
	}

	snapshot := coord.Snapshot()
	if len(snapshot.LatestRuns) != 1 {
		t.Fatalf("latest runs len = %d, want 1", len(snapshot.LatestRuns))
	}
	if snapshot.LatestRuns[0].Trigger != "repair" {
		t.Fatalf("latest run trigger = %q, want repair", snapshot.LatestRuns[0].Trigger)
	}
}
