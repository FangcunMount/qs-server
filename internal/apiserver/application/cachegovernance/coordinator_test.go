package cachegovernance

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func TestCoordinatorHandleManualWarmupReturnsStructuredResultAndRecordsRun(t *testing.T) {
	var scaleCalls []string
	var systemCalls []int64

	coord := NewCoordinator(Config{Enable: true}, Dependencies{
		Runtime: NewFamilyRuntime(
			&redisplane.Handle{Family: redisplane.FamilyStatic, AllowWarmup: true},
			&redisplane.Handle{Family: redisplane.FamilyQuery, AllowWarmup: true},
		),
		WarmScale: func(_ context.Context, code string) error {
			scaleCalls = append(scaleCalls, code)
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
			{Kind: "query.stats_system", Scope: "org:1"},
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
	if result.Items[0].Status != ManualWarmupItemStatusOK || result.Items[1].Status != ManualWarmupItemStatusOK {
		t.Fatalf("unexpected item statuses: %+v", result.Items)
	}
	if len(scaleCalls) != 1 || scaleCalls[0] != "s-001" {
		t.Fatalf("scale warmup calls = %v, want [s-001]", scaleCalls)
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

func TestCoordinatorHandleRepairCompleteStillUsesStructuredExecutor(t *testing.T) {
	var questionnaireCalls []string

	coord := NewCoordinator(Config{Enable: true}, Dependencies{
		Runtime: NewFamilyRuntime(
			&redisplane.Handle{Family: redisplane.FamilyQuery, AllowWarmup: true},
		),
		WarmStatsQuestionnaire: func(_ context.Context, orgID int64, code string) error {
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
