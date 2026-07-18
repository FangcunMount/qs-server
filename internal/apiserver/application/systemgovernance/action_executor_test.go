package systemgovernance

import (
	"context"
	"errors"
	"fmt"
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

func TestActionExecutorReplaysTerminalOutcomeFromFallbackWhenPrimaryCompleteFails(t *testing.T) {
	for _, errorCode := range []int{0, internalcode.ErrInvalidArgument, internalcode.ErrConflict, internalcode.ErrInternalServerError} {
		t.Run(baseerrors.ParseCoder(baseerrors.WithCode(errorCode, "outcome")).String(), func(t *testing.T) {
			governance := &fakeStatisticsGovernance{}
			if errorCode != 0 {
				governance.manualErr = baseerrors.WithCode(errorCode, "governance failure")
			}
			primary := &failingActionAudit{memoryActionAudit: memoryActionAudit{results: map[string]*ActionAuditReplay{}}, failComplete: true}
			fallback := &memoryActionAuditFallback{records: map[string]ActionAuditRecord{}}
			audit := NewRecoverableActionAuditStore(primary, fallback)
			audit.primaryRetryWindow = 10 * time.Millisecond
			audit.fallbackTimeout = time.Second
			executor := NewActionExecutorWithResilience(NewActionRegistry(), governance, nil, nil, audit)
			req := ActionRunRequest{RequestID: "fallback-request", Confirm: true, Input: map[string]interface{}{"targets": []interface{}{}}}

			firstResult, firstErr := executor.Run(context.Background(), 9, "cache.manual_warmup", req)
			secondResult, secondErr := executor.Run(context.Background(), 9, "cache.manual_warmup", req)
			if errorCode == 0 {
				if firstErr != nil || secondErr != nil || firstResult == nil || secondResult == nil || firstResult.Status != secondResult.Status {
					t.Fatalf("results=(%+v,%+v) errors=(%v,%v)", firstResult, secondResult, firstErr, secondErr)
				}
			} else if firstErr == nil || secondErr == nil || baseerrors.ParseCoder(firstErr).Code() != errorCode ||
				baseerrors.ParseCoder(secondErr).Code() != errorCode || firstErr.Error() != secondErr.Error() {
				t.Fatalf("first=%v second=%v, want replayed code=%d", firstErr, secondErr, errorCode)
			}
			if governance.manualCalls != 1 || primary.completeCalls < 1 {
				t.Fatalf("manualCalls=%d completeCalls=%d, want one execution and attempted completion", governance.manualCalls, primary.completeCalls)
			}
		})
	}
}

func TestActionAuditFallbackRejectsRequestIDForAnotherAction(t *testing.T) {
	fallback := &memoryActionAuditFallback{records: map[string]ActionAuditRecord{
		"9:shared-request": {OrgID: 9, RequestID: "shared-request", ActionID: "cache.repair_complete", Status: "ok", FinishedAt: time.Now()},
	}}
	audit := NewRecoverableActionAuditStore(&memoryActionAudit{results: map[string]*ActionAuditReplay{}}, fallback)
	executor := NewActionExecutorWithResilience(NewActionRegistry(), &fakeStatisticsGovernance{}, nil, nil, audit)
	_, err := executor.Run(context.Background(), 9, "cache.manual_warmup", ActionRunRequest{RequestID: "shared-request", Confirm: true})
	if err == nil || baseerrors.ParseCoder(err).Code() != internalcode.ErrConflict {
		t.Fatalf("Run() error=%v, want conflict", err)
	}
}

func TestActionAuditRecoveryBackfillsPrimaryAndDeletesFallback(t *testing.T) {
	primary := &failingActionAudit{memoryActionAudit: memoryActionAudit{results: map[string]*ActionAuditReplay{}}, failComplete: true}
	fallback := &memoryActionAuditFallback{records: map[string]ActionAuditRecord{}}
	audit := NewRecoverableActionAuditStore(primary, fallback)
	audit.primaryRetryWindow = 10 * time.Millisecond
	audit.fallbackTimeout = time.Second
	record := ActionAuditRecord{OrgID: 9, RequestID: "recover-request", ActionID: "cache.manual_warmup", Status: "ok", FinishedAt: time.Now(), Result: &ActionRunResult{ActionID: "cache.manual_warmup", Status: "ok"}}
	if _, claimed, err := audit.Claim(context.Background(), record); err != nil || !claimed {
		t.Fatalf("Claim() claimed=%v err=%v", claimed, err)
	}
	if err := audit.Complete(context.Background(), record); err != nil {
		t.Fatalf("Complete() error=%v", err)
	}
	primary.failComplete = false
	if recovered, err := audit.Recover(context.Background(), 100); err != nil || recovered != 1 {
		t.Fatalf("Recover() recovered=%d err=%v", recovered, err)
	}
	if _, exists, _ := fallback.Load(context.Background(), 9, record.RequestID); exists {
		t.Fatal("fallback was not deleted after recovery")
	}
	if replay := primary.results[record.RequestID]; replay == nil || replay.Result == nil {
		t.Fatalf("primary replay=%+v, want recovered result", replay)
	}
}

func TestActionExecutorReturnsFixedInternalErrorWhenBothAuditStoresFail(t *testing.T) {
	governance := &fakeStatisticsGovernance{}
	primary := &failingActionAudit{memoryActionAudit: memoryActionAudit{results: map[string]*ActionAuditReplay{}}, failComplete: true}
	audit := NewRecoverableActionAuditStore(primary, failingActionAuditFallback{})
	audit.primaryRetryWindow = 10 * time.Millisecond
	audit.fallbackTimeout = 10 * time.Millisecond
	executor := NewActionExecutorWithResilience(NewActionRegistry(), governance, nil, nil, audit)
	_, err := executor.Run(context.Background(), 9, "cache.manual_warmup", ActionRunRequest{RequestID: "dual-failure", Confirm: true, Input: map[string]interface{}{"targets": []interface{}{}}})
	if err == nil || baseerrors.ParseCoder(err).Code() != internalcode.ErrInternalServerError {
		t.Fatalf("Run() error=%v, want fixed internal error", err)
	}
	_, _ = executor.Run(context.Background(), 9, "cache.manual_warmup", ActionRunRequest{RequestID: "dual-failure", Confirm: true})
	if governance.manualCalls != 1 {
		t.Fatalf("manualCalls=%d, want no re-execution", governance.manualCalls)
	}
}

type memoryActionAudit struct {
	mu        sync.Mutex
	claims    int
	completes int
	running   bool
	results   map[string]*ActionAuditReplay
}

type failingActionAudit struct {
	memoryActionAudit
	failComplete  bool
	completeCalls int
}

func (a *failingActionAudit) Complete(ctx context.Context, record ActionAuditRecord) error {
	a.completeCalls++
	if a.failComplete {
		return errors.New("mysql complete failed")
	}
	return a.memoryActionAudit.Complete(ctx, record)
}

type memoryActionAuditFallback struct {
	mu      sync.Mutex
	records map[string]ActionAuditRecord
}

func (s *memoryActionAuditFallback) Load(_ context.Context, orgID int64, requestID string) (ActionAuditRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[controlAuditKey(orgID, requestID)]
	return record, ok, nil
}

func (s *memoryActionAuditFallback) Put(_ context.Context, record ActionAuditRecord) error {
	s.mu.Lock()
	s.records[controlAuditKey(record.OrgID, record.RequestID)] = record
	s.mu.Unlock()
	return nil
}

func (s *memoryActionAuditFallback) Delete(_ context.Context, orgID int64, requestID string) error {
	s.mu.Lock()
	delete(s.records, controlAuditKey(orgID, requestID))
	s.mu.Unlock()
	return nil
}

func (s *memoryActionAuditFallback) List(_ context.Context, limit int) ([]ActionAuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]ActionAuditRecord, 0, len(s.records))
	for _, record := range s.records {
		if len(result) == limit {
			break
		}
		result = append(result, record)
	}
	return result, nil
}

type failingActionAuditFallback struct{}

func (failingActionAuditFallback) Load(context.Context, int64, string) (ActionAuditRecord, bool, error) {
	return ActionAuditRecord{}, false, nil
}
func (failingActionAuditFallback) Put(context.Context, ActionAuditRecord) error {
	return errors.New("redis fallback failed")
}
func (failingActionAuditFallback) Delete(context.Context, int64, string) error { return nil }
func (failingActionAuditFallback) List(context.Context, int) ([]ActionAuditRecord, error) {
	return nil, nil
}

func controlAuditKey(orgID int64, requestID string) string {
	return fmt.Sprintf("%d:%s", orgID, requestID)
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
