package catalogreconcile

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeAuditStore struct {
	checkpoint AuditCheckpoint
	missing    bool
	bounds     AuditUpperBounds
	batches    []AuditBatchResult
	scanErr    error
	saveErr    error
	saves      int
	requests   []AuditBatchRequest
	indexErr   error
	indexCalls int
}

func (f *fakeAuditStore) VerifyAuditIndexes(context.Context) error {
	f.indexCalls++
	return f.indexErr
}
func (f *fakeAuditStore) LoadAuditCheckpoint(context.Context) (AuditCheckpoint, error) {
	if f.missing {
		return AuditCheckpoint{}, ErrAuditCheckpointMissing
	}
	return f.checkpoint, nil
}
func (f *fakeAuditStore) SaveAuditCheckpoint(_ context.Context, expected int64, checkpoint AuditCheckpoint) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if f.checkpoint.Revision != expected && !(f.missing && expected == 0) {
		return ErrAuditCheckpointCAS
	}
	f.missing = false
	f.checkpoint = checkpoint
	f.saves++
	return nil
}
func (f *fakeAuditStore) LoadAuditUpperBounds(context.Context, time.Duration) (AuditUpperBounds, error) {
	return f.bounds, nil
}
func (f *fakeAuditStore) ScanAuditBatch(_ context.Context, request AuditBatchRequest) (AuditBatchResult, error) {
	f.requests = append(f.requests, request)
	if f.scanErr != nil {
		return AuditBatchResult{}, f.scanErr
	}
	result := f.batches[0]
	f.batches = f.batches[1:]
	return result, nil
}

func TestAuditInitializesFixedUpperBoundsBeforeScanning(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{missing: true, bounds: AuditUpperBounds{SourceAssessmentID: 20, CatalogAssessmentID: 18}}
	service := NewService(&fakeStore{}, store)

	outcome, err := service.RunAuditBatch(context.Background(), auditTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.requests) != 0 || store.checkpoint.Phase != AuditPhaseMissing || store.checkpoint.SourceUpperAssessmentID != 20 || store.checkpoint.CatalogUpperAssessmentID != 18 {
		t.Fatalf("checkpoint/outcome = %#v / %#v", store.checkpoint, outcome)
	}
}

func TestAuditAdvancesCursorWhenHealthyBatchHasNoFindings(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{
		checkpoint: AuditCheckpoint{SchemaVersion: 1, Revision: 1, CycleID: "cycle-1", Phase: AuditPhaseMissing, SourceUpperAssessmentID: 20},
		batches:    []AuditBatchResult{{NextAssessmentID: 10, Scanned: 10, OrgCounts: map[int64]DriftCounts{}}},
	}
	service := NewService(&fakeStore{}, store)

	outcome, err := service.RunAuditBatch(context.Background(), auditTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Cursor != 10 || outcome.Findings != 0 || store.checkpoint.AfterAssessmentID != 10 || store.checkpoint.Revision != 2 {
		t.Fatalf("outcome/checkpoint = %#v / %#v", outcome, store.checkpoint)
	}
}

func TestAuditDoesNotAdvanceCheckpointWhenScanFails(t *testing.T) {
	t.Parallel()
	scanErr := errors.New("deadline exceeded")
	store := &fakeAuditStore{
		checkpoint: AuditCheckpoint{Revision: 7, CycleID: "cycle-1", Phase: AuditPhaseCatalog, AfterAssessmentID: 100, CatalogUpperAssessmentID: 200},
		scanErr:    scanErr,
	}
	service := NewService(&fakeStore{}, store)

	outcome, err := service.RunAuditBatch(context.Background(), auditTestOptions())
	if err == nil || !errors.Is(err, scanErr) {
		t.Fatalf("RunAuditBatch() error = %v", err)
	}
	if outcome.CycleID != "cycle-1" || outcome.Phase != AuditPhaseCatalog || outcome.Cursor != 100 || outcome.UpperBound != 200 {
		t.Fatalf("error outcome lost batch context: %#v", outcome)
	}
	if store.saves != 0 || store.checkpoint.Revision != 7 || store.checkpoint.AfterAssessmentID != 100 {
		t.Fatalf("checkpoint advanced after failure: %#v", store.checkpoint)
	}
}

func TestAuditFailsClosedAndRetriesVerificationWhenIndexesAreMissing(t *testing.T) {
	t.Parallel()
	indexErr := errors.New("required audit index is missing")
	store := &fakeAuditStore{
		checkpoint: AuditCheckpoint{Revision: 7, CycleID: "cycle-1", Phase: AuditPhaseMissing, SourceUpperAssessmentID: 200},
		indexErr:   indexErr,
	}
	service := NewService(&fakeStore{}, store)

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := service.RunAuditBatch(context.Background(), auditTestOptions()); err == nil || !errors.Is(err, indexErr) {
			t.Fatalf("RunAuditBatch() error = %v", err)
		}
	}
	if store.indexCalls != 2 || len(store.requests) != 0 || store.saves != 0 {
		t.Fatalf("missing indexes should prevent scanning and be retried: %#v", store)
	}
}

func TestAuditDoesNotAdvanceCheckpointOnCASConflict(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{
		checkpoint: AuditCheckpoint{Revision: 7, CycleID: "cycle-1", Phase: AuditPhaseMissing, SourceUpperAssessmentID: 200},
		batches:    []AuditBatchResult{{NextAssessmentID: 120, Scanned: 20, OrgCounts: map[int64]DriftCounts{}}},
		saveErr:    ErrAuditCheckpointCAS,
	}
	service := NewService(&fakeStore{}, store)
	if _, err := service.RunAuditBatch(context.Background(), auditTestOptions()); !errors.Is(err, ErrAuditCheckpointCAS) {
		t.Fatalf("RunAuditBatch() error = %v", err)
	}
	if store.checkpoint.Revision != 7 || store.checkpoint.AfterAssessmentID != 0 {
		t.Fatalf("checkpoint changed after CAS conflict: %#v", store.checkpoint)
	}
}

func TestAuditRestartResumesFromStoredCursorAndUpperBound(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{
		checkpoint: AuditCheckpoint{Revision: 3, CycleID: "cycle-1", Phase: AuditPhaseMissing, AfterAssessmentID: 100, SourceUpperAssessmentID: 500},
		bounds:     AuditUpperBounds{SourceAssessmentID: 999, CatalogAssessmentID: 999},
		batches:    []AuditBatchResult{{NextAssessmentID: 120, Scanned: 20, OrgCounts: map[int64]DriftCounts{}}},
	}
	service := NewService(&fakeStore{}, store)
	if _, err := service.RunAuditBatch(context.Background(), auditTestOptions()); err != nil {
		t.Fatal(err)
	}
	if len(store.requests) != 1 || store.requests[0].AfterAssessmentID != 100 || store.requests[0].UpperAssessmentID != 500 {
		t.Fatalf("scan request = %#v", store.requests)
	}
}

func TestAuditTransitionsFromMissingSourcesToCatalogEntries(t *testing.T) {
	t.Parallel()
	store := &fakeAuditStore{
		checkpoint: AuditCheckpoint{Revision: 3, CycleID: "cycle-1", Phase: AuditPhaseMissing, SourceUpperAssessmentID: 500, CatalogUpperAssessmentID: 450},
		batches: []AuditBatchResult{
			{NextAssessmentID: 500, Scanned: 10, Exhausted: true, OrgCounts: map[int64]DriftCounts{}},
			{NextAssessmentID: 20, Scanned: 20, OrgCounts: map[int64]DriftCounts{}},
		},
	}
	service := NewService(&fakeStore{}, store)
	if _, err := service.RunAuditBatch(context.Background(), auditTestOptions()); err != nil {
		t.Fatal(err)
	}
	if store.checkpoint.Phase != AuditPhaseCatalog || store.checkpoint.AfterAssessmentID != 0 {
		t.Fatalf("phase transition checkpoint = %#v", store.checkpoint)
	}
	if _, err := service.RunAuditBatch(context.Background(), auditTestOptions()); err != nil {
		t.Fatal(err)
	}
	if got := store.requests[1]; got.Phase != AuditPhaseCatalog || got.AfterAssessmentID != 0 || got.UpperAssessmentID != 450 {
		t.Fatalf("catalog scan request = %#v", got)
	}
}

func TestAuditPublishesCompletedOrgSnapshot(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	store := &fakeAuditStore{
		checkpoint: AuditCheckpoint{
			Revision: 2, CycleID: "cycle-1", Phase: AuditPhaseCatalog, CatalogUpperAssessmentID: 20,
			WorkingCounts: DriftCounts{Missing: 1}, WorkingOrgCounts: map[int64]DriftCounts{1: {Missing: 1}},
		},
		batches: []AuditBatchResult{{Scanned: 2, Exhausted: true, Counts: DriftCounts{Dangling: 2}, OrgCounts: map[int64]DriftCounts{1: {Dangling: 2}}}},
	}
	catalogService := NewService(&fakeStore{}, store)
	catalogService.(*service).now = func() time.Time { return now }

	outcome, err := catalogService.RunAuditBatch(context.Background(), auditTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Completed || store.checkpoint.LastCompleted == nil || store.checkpoint.NextCycleAt != now.Add(24*time.Hour) {
		t.Fatalf("completed checkpoint = %#v", store.checkpoint)
	}
	snapshot, err := catalogService.LatestAuditSnapshot(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CycleID != "cycle-1" || snapshot.Counts.Missing != 1 || snapshot.Counts.Dangling != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestAuditStartsDueCycleWithFreshUpperBoundsAndPreservesSnapshot(t *testing.T) {
	t.Parallel()
	completedAt := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	now := completedAt.Add(25 * time.Hour)
	lastCompleted := &CompletedAuditSnapshot{CycleID: "cycle-0", CompletedAt: completedAt, Counts: DriftCounts{Missing: 1}}
	store := &fakeAuditStore{
		checkpoint: AuditCheckpoint{
			Revision: 4, CycleID: "cycle-0", Phase: AuditPhaseCompleted,
			SourceUpperAssessmentID: 100, CatalogUpperAssessmentID: 90,
			LastCompleted: lastCompleted, NextCycleAt: completedAt.Add(24 * time.Hour),
		},
		bounds: AuditUpperBounds{SourceAssessmentID: 150, CatalogAssessmentID: 140},
	}
	catalogService := NewService(&fakeStore{}, store)
	catalogService.(*service).now = func() time.Time { return now }

	outcome, err := catalogService.RunAuditBatch(context.Background(), auditTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.CycleID == "cycle-0" || store.checkpoint.SourceUpperAssessmentID != 150 || store.checkpoint.CatalogUpperAssessmentID != 140 || store.checkpoint.LastCompleted != lastCompleted {
		t.Fatalf("new cycle checkpoint/outcome = %#v / %#v", store.checkpoint, outcome)
	}
}

func auditTestOptions() AuditRunOptions {
	return AuditRunOptions{BatchSize: 200, BatchTimeout: 3 * time.Second, CycleInterval: 24 * time.Hour}
}
