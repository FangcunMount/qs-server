package catalogreconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const (
	AuditCheckpointID     = "report_catalog"
	AuditCheckpointSchema = 1
	AuditPhaseMissing     = "missing_sources"
	AuditPhaseCatalog     = "catalog_entries"
	AuditPhaseCompleted   = "completed"
)

var (
	ErrAuditNotReady          = errors.New("catalog_audit_not_ready")
	ErrAuditCheckpointMissing = errors.New("catalog audit checkpoint is missing")
	ErrAuditCheckpointCAS     = errors.New("catalog audit checkpoint CAS conflict")
)

type AuditSnapshot struct {
	CycleID     string
	CompletedAt time.Time
	Counts      DriftCounts
}

type CompletedAuditSnapshot struct {
	CycleID     string
	CompletedAt time.Time
	Counts      DriftCounts
	OrgCounts   map[int64]DriftCounts
}

type AuditCheckpoint struct {
	SchemaVersion            int
	Revision                 int64
	CycleID                  string
	Phase                    string
	AfterAssessmentID        uint64
	SourceUpperAssessmentID  uint64
	CatalogUpperAssessmentID uint64
	WorkingCounts            DriftCounts
	WorkingOrgCounts         map[int64]DriftCounts
	LastCompleted            *CompletedAuditSnapshot
	NextCycleAt              time.Time
	UpdatedAt                time.Time
}

type AuditUpperBounds struct {
	SourceAssessmentID  uint64
	CatalogAssessmentID uint64
}

type AuditBatchRequest struct {
	CycleID           string
	Phase             string
	AfterAssessmentID uint64
	UpperAssessmentID uint64
	Limit             int
	MaxTime           time.Duration
}

type AuditBatchResult struct {
	NextAssessmentID uint64
	Scanned          int
	Exhausted        bool
	Counts           DriftCounts
	OrgCounts        map[int64]DriftCounts
}

type AuditBatchOutcome struct {
	CycleID     string
	Phase       string
	Cursor      uint64
	UpperBound  uint64
	Scanned     int
	Findings    int64
	Completed   bool
	Idle        bool
	NextCycleAt time.Time
}

type AuditRunOptions struct {
	BatchSize     int
	BatchTimeout  time.Duration
	CycleInterval time.Duration
}

type AuditStore interface {
	VerifyAuditIndexes(context.Context) error
	LoadAuditCheckpoint(context.Context) (AuditCheckpoint, error)
	SaveAuditCheckpoint(context.Context, int64, AuditCheckpoint) error
	LoadAuditUpperBounds(context.Context, time.Duration) (AuditUpperBounds, error)
	ScanAuditBatch(context.Context, AuditBatchRequest) (AuditBatchResult, error)
}

type RunnerService interface {
	RunAuditBatch(context.Context, AuditRunOptions) (AuditBatchOutcome, error)
}

func (s *service) RunAuditBatch(ctx context.Context, opts AuditRunOptions) (AuditBatchOutcome, error) {
	if s == nil || s.audit == nil {
		return AuditBatchOutcome{}, fmt.Errorf("catalog audit service is not configured")
	}
	if opts.BatchSize <= 0 || opts.BatchTimeout <= 0 || opts.CycleInterval <= 0 {
		return AuditBatchOutcome{}, fmt.Errorf("catalog audit run options are invalid")
	}
	checkpoint, err := s.audit.LoadAuditCheckpoint(ctx)
	missingCheckpoint := errors.Is(err, ErrAuditCheckpointMissing)
	if err != nil && !missingCheckpoint {
		return AuditBatchOutcome{}, fmt.Errorf("load catalog audit checkpoint: %w", err)
	}
	now := s.now().UTC()
	if err := s.verifyAuditIndexes(ctx); err != nil {
		observeAuditReady(false)
		observeAuditError("indexes")
		return AuditBatchOutcome{}, fmt.Errorf("catalog audit indexes unavailable: %w", err)
	}
	observeAuditReady(true)
	if !missingCheckpoint {
		observeAuditCheckpoint(checkpoint)
	}
	if !missingCheckpoint && checkpoint.Phase == AuditPhaseCompleted && now.Before(checkpoint.NextCycleAt) {
		return AuditBatchOutcome{CycleID: checkpoint.CycleID, Phase: checkpoint.Phase, Completed: true, Idle: true, NextCycleAt: checkpoint.NextCycleAt}, nil
	}
	if missingCheckpoint {
		return s.startAuditCycle(ctx, AuditCheckpoint{}, opts)
	}
	if checkpoint.Phase == AuditPhaseCompleted {
		return s.startAuditCycle(ctx, checkpoint, opts)
	}

	upper := checkpoint.SourceUpperAssessmentID
	if checkpoint.Phase == AuditPhaseCatalog {
		upper = checkpoint.CatalogUpperAssessmentID
	}
	outcome := AuditBatchOutcome{
		CycleID: checkpoint.CycleID, Phase: checkpoint.Phase,
		Cursor: checkpoint.AfterAssessmentID, UpperBound: upper,
	}
	batchStartedAt := time.Now()
	result, err := s.audit.ScanAuditBatch(ctx, AuditBatchRequest{
		CycleID: checkpoint.CycleID, Phase: checkpoint.Phase,
		AfterAssessmentID: checkpoint.AfterAssessmentID, UpperAssessmentID: upper,
		Limit: opts.BatchSize, MaxTime: opts.BatchTimeout,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			observeAuditError("timeout")
		} else {
			observeAuditError("scan")
		}
		observeAuditExecution(checkpoint.Phase, "error", time.Since(batchStartedAt).Seconds())
		return outcome, fmt.Errorf("scan catalog audit batch: %w", err)
	}
	observeAuditExecution(checkpoint.Phase, "success", time.Since(batchStartedAt).Seconds())
	if err := ctx.Err(); err != nil {
		outcome.Scanned = result.Scanned
		outcome.Findings = result.Counts.Total()
		return outcome, err
	}

	next := checkpoint
	next.Revision = checkpoint.Revision + 1
	next.UpdatedAt = now
	next.WorkingCounts = addDriftCounts(next.WorkingCounts, result.Counts)
	if next.WorkingOrgCounts == nil {
		next.WorkingOrgCounts = make(map[int64]DriftCounts)
	}
	for orgID, counts := range result.OrgCounts {
		next.WorkingOrgCounts[orgID] = addDriftCounts(next.WorkingOrgCounts[orgID], counts)
	}
	next.AfterAssessmentID = result.NextAssessmentID
	completed := false
	if result.Exhausted {
		switch checkpoint.Phase {
		case AuditPhaseMissing:
			next.Phase = AuditPhaseCatalog
			next.AfterAssessmentID = 0
		case AuditPhaseCatalog:
			completed = true
			next.Phase = AuditPhaseCompleted
			next.AfterAssessmentID = 0
			next.LastCompleted = &CompletedAuditSnapshot{
				CycleID: checkpoint.CycleID, CompletedAt: now,
				Counts: next.WorkingCounts, OrgCounts: cloneOrgCounts(next.WorkingOrgCounts),
			}
			next.NextCycleAt = now.Add(opts.CycleInterval)
		default:
			return AuditBatchOutcome{}, fmt.Errorf("unknown catalog audit phase %q", checkpoint.Phase)
		}
	}
	if err := s.audit.SaveAuditCheckpoint(ctx, checkpoint.Revision, next); err != nil {
		observeAuditError("checkpoint_cas")
		outcome.Scanned = result.Scanned
		outcome.Findings = result.Counts.Total()
		return outcome, err
	}
	observeAuditCheckpoint(next)
	observeAuditBatch(checkpoint.Phase, result.Scanned, result.Counts.Total(), completed)
	return AuditBatchOutcome{
		CycleID: checkpoint.CycleID, Phase: checkpoint.Phase,
		Cursor: result.NextAssessmentID, UpperBound: upper,
		Scanned: result.Scanned, Findings: result.Counts.Total(), Completed: completed, NextCycleAt: next.NextCycleAt,
	}, nil
}

func (s *service) verifyAuditIndexes(ctx context.Context) error {
	s.auditIndexMu.Lock()
	defer s.auditIndexMu.Unlock()
	if s.auditIndexesReady {
		return nil
	}
	if err := s.audit.VerifyAuditIndexes(ctx); err != nil {
		return err
	}
	s.auditIndexesReady = true
	return nil
}

func (s *service) startAuditCycle(ctx context.Context, previous AuditCheckpoint, opts AuditRunOptions) (AuditBatchOutcome, error) {
	bounds, err := s.audit.LoadAuditUpperBounds(ctx, opts.BatchTimeout)
	if err != nil {
		return AuditBatchOutcome{}, fmt.Errorf("load catalog audit upper bounds: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return AuditBatchOutcome{}, err
	}
	now := s.now().UTC()
	cycleID := meta.New().String()
	next := AuditCheckpoint{
		SchemaVersion: AuditCheckpointSchema, Revision: previous.Revision + 1,
		CycleID: cycleID, Phase: AuditPhaseMissing,
		SourceUpperAssessmentID:  bounds.SourceAssessmentID,
		CatalogUpperAssessmentID: bounds.CatalogAssessmentID,
		WorkingOrgCounts:         make(map[int64]DriftCounts), LastCompleted: previous.LastCompleted,
		UpdatedAt: now,
	}
	if err := s.audit.SaveAuditCheckpoint(ctx, previous.Revision, next); err != nil {
		return AuditBatchOutcome{}, err
	}
	observeAuditCheckpoint(next)
	return AuditBatchOutcome{CycleID: cycleID, Phase: AuditPhaseMissing, UpperBound: bounds.SourceAssessmentID}, nil
}

func (s *service) LatestAuditSnapshot(ctx context.Context, orgID int64) (AuditSnapshot, error) {
	if s == nil || s.audit == nil || orgID == 0 {
		return AuditSnapshot{}, ErrAuditNotReady
	}
	checkpoint, err := s.audit.LoadAuditCheckpoint(ctx)
	if errors.Is(err, ErrAuditCheckpointMissing) {
		return AuditSnapshot{}, ErrAuditNotReady
	}
	if err != nil {
		return AuditSnapshot{}, err
	}
	if checkpoint.LastCompleted == nil {
		return AuditSnapshot{}, ErrAuditNotReady
	}
	return AuditSnapshot{
		CycleID:     checkpoint.LastCompleted.CycleID,
		CompletedAt: checkpoint.LastCompleted.CompletedAt,
		Counts:      checkpoint.LastCompleted.OrgCounts[orgID],
	}, nil
}

func addDriftCounts(left, right DriftCounts) DriftCounts {
	return DriftCounts{
		Missing:             left.Missing + right.Missing,
		Dangling:            left.Dangling + right.Dangling,
		AssociationMismatch: left.AssociationMismatch + right.AssociationMismatch,
		WrongWinner:         left.WrongWinner + right.WrongWinner,
	}
}

func cloneOrgCounts(source map[int64]DriftCounts) map[int64]DriftCounts {
	result := make(map[int64]DriftCounts, len(source))
	for orgID, counts := range source {
		result[orgID] = counts
	}
	return result
}
