package scheduler

import (
	"context"
	"errors"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"reflect"
	"testing"
	"time"

	evaluationscheduler "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

type fakeEvaluationConsistencyAuditService struct {
	results        map[uint64]evaluationscheduler.AuditBatchResult
	after          []uint64
	err            error
	reverseErr     error
	reverseCalls   int
	reverseUpper   []uint64
	reverseResults map[uint64]evaluationscheduler.AuditBatchResult
}

func (f *fakeEvaluationConsistencyAuditService) AuditBatch(_ context.Context, after uint64, _ int) (evaluationscheduler.AuditBatchResult, error) {
	f.after = append(f.after, after)
	if f.err != nil {
		return evaluationscheduler.AuditBatchResult{}, f.err
	}
	return f.results[after], nil
}

func (f *fakeEvaluationConsistencyAuditService) BusinessUpperBound(context.Context) (uint64, error) {
	return 120, nil
}
func (f *fakeEvaluationConsistencyAuditService) OutboxUpperBound(context.Context) (uint64, error) {
	return 33, nil
}
func (f *fakeEvaluationConsistencyAuditService) AuditBatchTo(ctx context.Context, after, upper uint64, limit int) (evaluationscheduler.AuditBatchResult, error) {
	return f.AuditBatch(ctx, after, limit)
}
func (f *fakeEvaluationConsistencyAuditService) AuditOutboxBatch(_ context.Context, after, upper uint64, _ int) (evaluationscheduler.AuditBatchResult, error) {
	f.reverseCalls++
	f.reverseUpper = append(f.reverseUpper, upper)
	if f.reverseErr != nil {
		return evaluationscheduler.AuditBatchResult{}, f.reverseErr
	}
	if f.reverseResults == nil {
		return evaluationscheduler.AuditBatchResult{CycleComplete: true}, nil
	}
	return f.reverseResults[after], nil
}

func TestEvaluationConsistencyAuditExecutesOneCompleteWatermarkedCycle(t *testing.T) {
	service := &fakeEvaluationConsistencyAuditService{results: map[uint64]evaluationscheduler.AuditBatchResult{
		0:   {Scanned: 100, NextCursor: 100},
		100: {Scanned: 20, NextCursor: 120, CycleComplete: true},
	}}
	opts := newTestEvaluationConsistencyAuditOptions()
	opts.BatchInterval = time.Nanosecond
	now := time.Now()
	runner := &EvaluationConsistencyAuditRunner{opts: opts, service: service, now: func() time.Time {
		now = now.Add(time.Second)
		return now
	}}

	if err := runner.executeCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []uint64{0, 100}; !reflect.DeepEqual(service.after, want) {
		t.Fatalf("audit watermarks = %v, want %v", service.after, want)
	}
}

func TestEvaluationConsistencyAuditRejectsStalledWatermark(t *testing.T) {
	service := &fakeEvaluationConsistencyAuditService{results: map[uint64]evaluationscheduler.AuditBatchResult{
		0: {Scanned: 100, NextCursor: 0},
	}}
	runner := &EvaluationConsistencyAuditRunner{opts: newTestEvaluationConsistencyAuditOptions(), service: service, now: time.Now}
	if err := runner.executeCycle(context.Background()); err == nil {
		t.Fatal("expected stalled watermark error")
	}
}

func TestEvaluationConsistencyAuditCompletesAfterExactSizeBatch(t *testing.T) {
	service := &fakeEvaluationConsistencyAuditService{results: map[uint64]evaluationscheduler.AuditBatchResult{
		0:   {Scanned: 100, NextCursor: 100},
		100: {CycleComplete: true},
	}}
	opts := newTestEvaluationConsistencyAuditOptions()
	opts.BatchInterval = time.Nanosecond
	runner := &EvaluationConsistencyAuditRunner{opts: opts, service: service, now: time.Now}

	if err := runner.executeCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []uint64{0, 100}; !reflect.DeepEqual(service.after, want) {
		t.Fatalf("audit watermarks = %v, want %v", service.after, want)
	}
}

func TestEvaluationConsistencyAuditPropagatesBatchFailure(t *testing.T) {
	runner := &EvaluationConsistencyAuditRunner{
		opts:    newTestEvaluationConsistencyAuditOptions(),
		service: &fakeEvaluationConsistencyAuditService{err: errors.New("read failed")},
		now:     time.Now,
	}
	if err := runner.executeCycle(context.Background()); err == nil {
		t.Fatal("expected batch failure")
	}
}

func newTestEvaluationConsistencyAuditOptions() *apiserveroptions.EvaluationConsistencyAuditOptions {
	return &apiserveroptions.EvaluationConsistencyAuditOptions{
		Enable: true, InitialDelay: 0, BatchInterval: time.Millisecond, CycleInterval: 24 * time.Hour,
		BatchSize: 100, BatchTimeout: time.Second, LockKey: "qs:evaluation-consistency-audit:test", LockTTL: 30 * time.Second,
	}
}

var _ evaluationscheduler.Service = (*fakeEvaluationConsistencyAuditService)(nil)

func TestEvaluationConsistencyAuditRejectsIncompleteReverseCycle(t *testing.T) {
	service := &fakeEvaluationConsistencyAuditService{results: map[uint64]evaluationscheduler.AuditBatchResult{0: {CycleComplete: true}}, reverseErr: errors.New("reverse read failed")}
	runner := &EvaluationConsistencyAuditRunner{opts: newTestEvaluationConsistencyAuditOptions(), service: service, now: time.Now}
	before := testutil.ToFloat64(evaluationConsistencyAuditLastSuccess)
	if err := runner.executeCycle(context.Background()); err == nil {
		t.Fatal("reverse failure claimed success")
	}
	if service.reverseCalls != 1 || testutil.ToFloat64(evaluationConsistencyAuditLastSuccess) != before {
		t.Fatal("incomplete reverse cycle advanced success timestamp")
	}
}
func TestEvaluationConsistencyAuditKeepsFixedReverseUpperBound(t *testing.T) {
	service := &fakeEvaluationConsistencyAuditService{results: map[uint64]evaluationscheduler.AuditBatchResult{0: {CycleComplete: true}}, reverseResults: map[uint64]evaluationscheduler.AuditBatchResult{0: {Scanned: 2, NextCursor: 20}, 20: {Scanned: 1, NextCursor: 33, CycleComplete: true}}}
	opts := newTestEvaluationConsistencyAuditOptions()
	opts.BatchInterval = time.Nanosecond
	runner := &EvaluationConsistencyAuditRunner{opts: opts, service: service, now: time.Now}
	if err := runner.executeCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(service.reverseUpper, []uint64{33, 33}) {
		t.Fatalf("reverse upper bounds=%v", service.reverseUpper)
	}
}
