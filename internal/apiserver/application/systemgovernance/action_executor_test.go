package systemgovernance

import (
	"context"
	"testing"
	"time"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
)

func TestActionExecutorRequiresConfirmForManualWarmup(t *testing.T) {
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{})
	_, err := executor.Run(context.Background(), 9, "cache.manual_warmup", ActionRunRequest{
		Input: map[string]interface{}{"targets": []interface{}{}},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want confirmation error")
	}
}

func TestActionExecutorRunsManualWarmupWithConfirmedInput(t *testing.T) {
	governance := &fakeStatisticsGovernance{}
	executor := NewActionExecutor(NewActionRegistry(), governance)
	result, err := executor.Run(context.Background(), 9, "cache.manual_warmup", ActionRunRequest{
		Confirm: true,
		Input: map[string]interface{}{
			"targets": []interface{}{
				map[string]interface{}{"kind": "static.scale", "scope": "scale:S-001"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != "ok" || governance.manualOrgID != 9 {
		t.Fatalf("result = %#v manualOrgID=%d, want ok for org 9", result, governance.manualOrgID)
	}
	if len(governance.manualReq.Targets) != 1 || governance.manualReq.Targets[0].Scope != "scale:S-001" {
		t.Fatalf("manual request = %#v", governance.manualReq)
	}
}

func TestActionExecutorRejectsPlannedAction(t *testing.T) {
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{})
	_, err := executor.Run(context.Background(), 9, "events.replay", ActionRunRequest{Confirm: true})
	if err == nil {
		t.Fatal("Run() error = nil, want disabled action error")
	}
}

type fakeStatisticsGovernance struct {
	manualOrgID int64
	manualReq   statisticsApp.ManualWarmupRequest
	repairOrgID int64
	repairReq   statisticsApp.RepairCompleteRequest
}

func (f *fakeStatisticsGovernance) TriggerStatisticsWarmup(context.Context, int64, string) {}

func (f *fakeStatisticsGovernance) HandleRepairComplete(_ context.Context, orgID int64, req statisticsApp.RepairCompleteRequest) error {
	f.repairOrgID = orgID
	f.repairReq = req
	return nil
}

func (f *fakeStatisticsGovernance) HandleManualWarmup(_ context.Context, orgID int64, req statisticsApp.ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error) {
	f.manualOrgID = orgID
	f.manualReq = req
	now := time.Now()
	return &cachemodel.ManualWarmupResult{
		Trigger:    "manual",
		StartedAt:  now,
		FinishedAt: now,
		Summary: cachemodel.ManualWarmupSummary{
			TargetCount: len(req.Targets),
			Result:      "ok",
		},
	}, nil
}

func (f *fakeStatisticsGovernance) GetStatus(context.Context) (*cachemodel.StatusSnapshot, error) {
	return nil, nil
}

func (f *fakeStatisticsGovernance) GetHotset(context.Context, string, string) (*statisticsApp.GovernanceHotsetResponse, error) {
	return nil, nil
}
