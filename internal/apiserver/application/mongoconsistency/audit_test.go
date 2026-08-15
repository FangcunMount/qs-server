package mongoconsistency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

type scannerStub struct {
	requests []BatchRequest
	upper    map[Phase]uint64
	findings map[Phase][]Finding
	err      error
}

func (s *scannerStub) UpperBound(_ context.Context, phase Phase, _ time.Duration) (uint64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.upper[phase], nil
}

func (s *scannerStub) ScanBatch(_ context.Context, request BatchRequest) (BatchResult, error) {
	s.requests = append(s.requests, request)
	if s.err != nil {
		return BatchResult{}, s.err
	}
	return BatchResult{NextID: request.UpperBound, Scanned: 1, Exhausted: true, Findings: s.findings[request.Phase]}, nil
}

type memoryCheckpoint struct {
	checkpoint Checkpoint
	exists     bool
	cas        bool
}

func (s *memoryCheckpoint) Load(context.Context) (Checkpoint, error) {
	if !s.exists {
		return Checkpoint{}, ErrCheckpointMissing
	}
	return s.checkpoint, nil
}

func (s *memoryCheckpoint) Save(_ context.Context, expected int64, checkpoint Checkpoint) error {
	if s.cas || (s.exists && s.checkpoint.Revision != expected) || (!s.exists && expected != 0) {
		return ErrCheckpointCAS
	}
	s.checkpoint, s.exists = checkpoint, true
	return nil
}

func TestAuditRunsAllBoundedPhasesAndPersistsCompletedStatistics(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	scanner := &scannerStub{upper: map[Phase]uint64{}, findings: map[Phase][]Finding{}}
	allKinds := make([]string, 0, len(DriftSeverities))
	for kind := range DriftSeverities {
		allKinds = append(allKinds, kind)
	}
	for index, phase := range AuditPhases {
		scanner.upper[phase] = uint64(index + 10)
		if index < len(allKinds) {
			scanner.findings[phase] = []Finding{{Kind: allKinds[index], Severity: DriftSeverities[allKinds[index]], SampleID: "internal-1"}}
		}
	}
	checkpoint := &memoryCheckpoint{}
	service := NewService(scanner, checkpoint)
	service.now = func() time.Time { return now }
	opts := RunOptions{BatchSize: 200, BatchTimeout: 3 * time.Second, CycleInterval: 24 * time.Hour, MaxSamples: 2}

	// First call freezes the first upper bound without scanning it.
	if outcome, err := service.RunAuditBatch(t.Context(), opts); err != nil || outcome.Phase != AuditPhases[0] {
		t.Fatalf("start cycle = %#v err=%v", outcome, err)
	}
	var completed BatchOutcome
	for range AuditPhases {
		var err error
		completed, err = service.RunAuditBatch(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !completed.Completed || checkpoint.checkpoint.Phase != PhaseCompleted || checkpoint.checkpoint.LastCompleted == nil {
		t.Fatalf("completed outcome=%#v checkpoint=%#v", completed, checkpoint.checkpoint)
	}
	if checkpoint.checkpoint.NextCycleAt != now.Add(24*time.Hour) {
		t.Fatalf("next cycle = %s", checkpoint.checkpoint.NextCycleAt)
	}
	if len(scanner.requests) != len(AuditPhases) || checkpoint.checkpoint.LastCompleted.Statistics.Scanned != int64(len(AuditPhases)) {
		t.Fatalf("requests=%d scanned=%d", len(scanner.requests), checkpoint.checkpoint.LastCompleted.Statistics.Scanned)
	}
	for index, request := range scanner.requests {
		if request.UpperBound != uint64(index+10) || request.Limit != 200 || request.MaxTime != 3*time.Second {
			t.Fatalf("request[%d] = %#v", index, request)
		}
	}
}

func TestAuditResumesCheckpointAcrossServiceRestart(t *testing.T) {
	scanner := &scannerStub{upper: map[Phase]uint64{AuditPhases[0]: 20}, findings: map[Phase][]Finding{}}
	store := &memoryCheckpoint{}
	opts := RunOptions{BatchSize: 1, BatchTimeout: time.Second, CycleInterval: time.Hour, MaxSamples: 1}
	first := NewService(scanner, store)
	first.now = func() time.Time { return time.Unix(100, 0).UTC() }
	if _, err := first.RunAuditBatch(t.Context(), opts); err != nil {
		t.Fatal(err)
	}

	restarted := NewService(scanner, store)
	restarted.now = first.now
	if _, err := restarted.RunAuditBatch(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	if len(scanner.requests) != 1 || scanner.requests[0].AfterID != 0 || scanner.requests[0].UpperBound != 20 {
		t.Fatalf("resumed requests = %#v", scanner.requests)
	}
}

func TestAuditRestoresCompletedMetricsAfterProcessRestart(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	completedAt := now.Add(-time.Hour)
	store := &memoryCheckpoint{exists: true, checkpoint: Checkpoint{
		SchemaVersion: CheckpointSchemaVersion,
		Revision:      3,
		CycleID:       "completed-cycle",
		Phase:         PhaseCompleted,
		NextCycleAt:   now.Add(23 * time.Hour),
		LastCompleted: &CompletedCycle{
			CycleID:     "completed-cycle",
			CompletedAt: completedAt,
			Statistics: Statistics{Findings: map[string]int64{
				DriftGenerationMissingRun: 2,
			}},
		},
	}}
	auditLastSuccess.Set(0)
	auditDrift.WithLabelValues(string(SeverityHigh), DriftGenerationMissingRun).Set(0)
	service := NewService(&scannerStub{}, store)
	service.now = func() time.Time { return now }

	outcome, err := service.RunAuditBatch(t.Context(), RunOptions{BatchSize: 1, BatchTimeout: time.Second, CycleInterval: 24 * time.Hour})
	if err != nil || !outcome.Idle {
		t.Fatalf("restart outcome=%#v err=%v", outcome, err)
	}
	if got := testutil.ToFloat64(auditLastSuccess); got != float64(completedAt.Unix()) {
		t.Fatalf("last success metric = %f, want %d", got, completedAt.Unix())
	}
	if got := testutil.ToFloat64(auditDrift.WithLabelValues(string(SeverityHigh), DriftGenerationMissingRun)); got != 2 {
		t.Fatalf("restored drift metric = %f, want 2", got)
	}
}

func TestAuditRecordsCancelledBatchAsExecutionError(t *testing.T) {
	store := &memoryCheckpoint{exists: true, checkpoint: Checkpoint{
		SchemaVersion: CheckpointSchemaVersion, Revision: 1, CycleID: "cycle",
		Phase: PhaseAnswerSheetOutbox, UpperBound: 1, Working: NewStatistics(),
	}}
	service := NewService(&scannerStub{findings: map[Phase][]Finding{}}, store)
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }
	before := testutil.ToFloat64(auditErrors.WithLabelValues("batch_context"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.RunAuditBatch(ctx, RunOptions{BatchSize: 1, BatchTimeout: time.Second, CycleInterval: time.Hour}); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunAuditBatch() error = %v, want context canceled", err)
	}
	if delta := testutil.ToFloat64(auditErrors.WithLabelValues("batch_context")) - before; delta != 1 {
		t.Fatalf("batch context error metric delta = %f, want 1", delta)
	}
}

func TestAuditFailsClosedOnCheckpointCAS(t *testing.T) {
	scanner := &scannerStub{upper: map[Phase]uint64{AuditPhases[0]: 1}, findings: map[Phase][]Finding{}}
	store := &memoryCheckpoint{cas: true}
	service := NewService(scanner, store)
	_, err := service.RunAuditBatch(t.Context(), RunOptions{BatchSize: 1, BatchTimeout: time.Second, CycleInterval: time.Hour})
	if !errors.Is(err, ErrCheckpointCAS) {
		t.Fatalf("RunAuditBatch() error = %v, want checkpoint CAS", err)
	}
}

func TestParseScopesRejectsUnknownAndDeduplicates(t *testing.T) {
	got, err := ParseScopes([]string{string(PhaseRetryOutbox), string(PhaseRetryOutbox)})
	if err != nil || len(got) != 1 || got[0] != PhaseRetryOutbox {
		t.Fatalf("ParseScopes() = %#v, %v", got, err)
	}
	if _, err := ParseScopes([]string{"repair"}); err == nil {
		t.Fatal("ParseScopes(repair) error = nil")
	}
}
