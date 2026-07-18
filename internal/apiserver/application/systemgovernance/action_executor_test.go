package systemgovernance

import (
	"context"
	"sync"
	"testing"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	internalcode "github.com/FangcunMount/qs-server/internal/pkg/code"
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

func TestActionExecutorRequestIDIsClaimedAndReplayedFromAudit(t *testing.T) {
	governance := &fakeStatisticsGovernance{}
	audit := &memoryActionAudit{results: map[string]*ActionAuditReplay{}}
	executor := NewActionExecutorWithResilience(NewActionRegistry(), governance, nil, nil, audit)
	req := ActionRunRequest{
		RequestID: "governance-request-1", Confirm: true,
		Input: map[string]interface{}{"targets": []interface{}{map[string]interface{}{"kind": "static.scale", "scope": "scale:S-001"}}},
	}
	first, err := executor.Run(context.Background(), 9, "cache.manual_warmup", req)
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	second, err := executor.Run(context.Background(), 9, "cache.manual_warmup", req)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if audit.claims != 2 || audit.completes != 1 || first.RequestID != req.RequestID || second.RequestID != req.RequestID {
		t.Fatalf("claims=%d completes=%d first=%+v second=%+v", audit.claims, audit.completes, first, second)
	}
}

func TestActionExecutorReplaysApplicationErrorsWithoutExecutingAgain(t *testing.T) {
	for _, errorCode := range []int{internalcode.ErrInvalidArgument, internalcode.ErrConflict, internalcode.ErrInternalServerError} {
		t.Run(baseerrors.ParseCoder(baseerrors.WithCode(errorCode, "failure")).String(), func(t *testing.T) {
			governance := &fakeStatisticsGovernance{manualErr: baseerrors.WithCode(errorCode, "governance failure")}
			audit := &memoryActionAudit{results: map[string]*ActionAuditReplay{}}
			executor := NewActionExecutorWithResilience(NewActionRegistry(), governance, nil, nil, audit)
			req := ActionRunRequest{RequestID: "failed-request", Confirm: true, Input: map[string]interface{}{"targets": []interface{}{}}}

			_, firstErr := executor.Run(context.Background(), 9, "cache.manual_warmup", req)
			_, secondErr := executor.Run(context.Background(), 9, "cache.manual_warmup", req)
			if firstErr == nil || secondErr == nil {
				t.Fatalf("errors = (%v, %v), want replayed errors", firstErr, secondErr)
			}
			if baseerrors.ParseCoder(firstErr).Code() != errorCode || baseerrors.ParseCoder(secondErr).Code() != errorCode || firstErr.Error() != secondErr.Error() {
				t.Fatalf("first=%v second=%v, want same code/message", firstErr, secondErr)
			}
			if governance.manualCalls != 1 {
				t.Fatalf("manual calls=%d, want 1", governance.manualCalls)
			}
		})
	}
}

type memoryActionAudit struct {
	mu        sync.Mutex
	claims    int
	completes int
	running   bool
	results   map[string]*ActionAuditReplay
}

func (a *memoryActionAudit) Claim(_ context.Context, record ActionAuditRecord) (*ActionAuditReplay, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.claims++
	if result := a.results[record.RequestID]; result != nil {
		return result, false, nil
	}
	if a.running {
		return nil, false, nil
	}
	a.running = true
	return nil, true, nil
}

func (a *memoryActionAudit) Complete(_ context.Context, record ActionAuditRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.completes++
	a.running = false
	a.results[record.RequestID] = &ActionAuditReplay{ActionID: record.ActionID, Result: record.Result, Error: record.Error}
	return nil
}

type fakeStatisticsGovernance struct {
	manualOrgID int64
	manualReq   statisticsApp.ManualWarmupRequest
	repairOrgID int64
	repairReq   statisticsApp.RepairCompleteRequest
	manualErr   error
	manualCalls int
}

func (f *fakeStatisticsGovernance) TriggerStatisticsWarmup(context.Context, int64, string) {}

func (f *fakeStatisticsGovernance) HandleRepairComplete(_ context.Context, orgID int64, req statisticsApp.RepairCompleteRequest) error {
	f.repairOrgID = orgID
	f.repairReq = req
	return nil
}

func (f *fakeStatisticsGovernance) HandleManualWarmup(_ context.Context, orgID int64, req statisticsApp.ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error) {
	f.manualCalls++
	if f.manualErr != nil {
		return nil, f.manualErr
	}
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
