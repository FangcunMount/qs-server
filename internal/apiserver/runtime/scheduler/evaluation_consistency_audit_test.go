package scheduler

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	evaluationscheduler "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

type fakeEvaluationConsistencyAuditService struct {
	results map[uint64]evaluationscheduler.AuditBatchResult
	after   []uint64
	err     error
}

func (f *fakeEvaluationConsistencyAuditService) AuditBatch(_ context.Context, after uint64, _ int) (evaluationscheduler.AuditBatchResult, error) {
	f.after = append(f.after, after)
	if f.err != nil {
		return evaluationscheduler.AuditBatchResult{}, f.err
	}
	return f.results[after], nil
}

func TestEvaluationConsistencyAuditExecutesOneCompleteWatermarkedCycle(t *testing.T) {
	service := &fakeEvaluationConsistencyAuditService{results: map[uint64]evaluationscheduler.AuditBatchResult{
		0:  {Scanned: 100, NextCursor: 100},
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
		opts: newTestEvaluationConsistencyAuditOptions(),
		service: &fakeEvaluationConsistencyAuditService{err: errors.New("read failed")},
		now: time.Now,
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
