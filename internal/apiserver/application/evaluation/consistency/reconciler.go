package consistency

import (
	"context"
	"fmt"
	"time"

	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// MismatchKind 标识跨存储不一致类型。
type MismatchKind string

const (
	MismatchReportWithoutInterpretedStatus        MismatchKind = "report_without_interpreted_status"
	MismatchScoringArtifactWithoutEvaluatedStatus MismatchKind = "scoring_artifact_without_evaluated_status"
)

// Mismatch 描述一次检测到的跨存储终态漂移。
type Mismatch struct {
	AssessmentID uint64
	Kind         MismatchKind
	DetectedAt   time.Time
}

// AssessmentStatusReader 读取测评当前持久化状态。
type AssessmentStatusReader interface {
	FindByID(ctx context.Context, id assessment.ID) (*assessment.Assessment, error)
}

// ReportExistenceChecker 判断 Mongo 报告是否已出站。
type ReportExistenceChecker interface {
	ReportExists(ctx context.Context, assessmentID uint64) (bool, error)
}

// ScoringArtifactChecker 判断计分产物是否已落库（快照或分数）。
type ScoringArtifactChecker interface {
	HasScoringArtifact(ctx context.Context, assessmentID uint64) (bool, error)
}

// Reconciler 扫描并修复 scoring/reporting 跨库部分成功窗口。
type Reconciler struct {
	assessments     AssessmentStatusReader
	reports         ReportExistenceChecker
	artifacts       ScoringArtifactChecker
	snapshotStore   outcomescoring.SnapshotStore
	assessmentSaver assessment.Repository
}

// NewReconciler 创建跨存储对账器。
func NewReconciler(
	assessments AssessmentStatusReader,
	reports ReportExistenceChecker,
	artifacts ScoringArtifactChecker,
	snapshotStore outcomescoring.SnapshotStore,
	assessmentSaver assessment.Repository,
) *Reconciler {
	return &Reconciler{
		assessments:     assessments,
		reports:         reports,
		artifacts:       artifacts,
		snapshotStore:   snapshotStore,
		assessmentSaver: assessmentSaver,
	}
}

// Scan 对给定 assessment 列表执行只读对账。
func (r *Reconciler) Scan(ctx context.Context, assessmentIDs []uint64) ([]Mismatch, error) {
	if r == nil {
		return nil, fmt.Errorf("consistency reconciler is not configured")
	}
	now := time.Now()
	out := make([]Mismatch, 0, len(assessmentIDs))
	for _, assessmentID := range assessmentIDs {
		if assessmentID == 0 {
			continue
		}
		mismatches, err := r.scanOne(ctx, assessmentID, now)
		if err != nil {
			return nil, err
		}
		out = append(out, mismatches...)
	}
	return out, nil
}

func (r *Reconciler) scanOne(ctx context.Context, assessmentID uint64, detectedAt time.Time) ([]Mismatch, error) {
	a, err := r.assessments.FindByID(ctx, assessment.NewID(assessmentID))
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}
	var mismatches []Mismatch
	if r.reports != nil && !a.Status().IsInterpreted() {
		exists, err := r.reports.ReportExists(ctx, assessmentID)
		if err != nil {
			return nil, err
		}
		if exists {
			mismatches = append(mismatches, Mismatch{
				AssessmentID: assessmentID,
				Kind:         MismatchReportWithoutInterpretedStatus,
				DetectedAt:   detectedAt,
			})
		}
	}
	if r.artifacts != nil && a.Status().IsSubmitted() {
		hasArtifact, err := r.artifacts.HasScoringArtifact(ctx, assessmentID)
		if err != nil {
			return nil, err
		}
		if hasArtifact {
			mismatches = append(mismatches, Mismatch{
				AssessmentID: assessmentID,
				Kind:         MismatchScoringArtifactWithoutEvaluatedStatus,
				DetectedAt:   detectedAt,
			})
		}
	}
	return mismatches, nil
}

// RepairInterpretedFinalization 幂等重放 reporting writer 末步：ApplyOutcome + assessment Save。
func (r *Reconciler) RepairInterpretedFinalization(ctx context.Context, assessmentID uint64) error {
	if r == nil || r.assessments == nil || r.assessmentSaver == nil || r.snapshotStore == nil {
		return fmt.Errorf("consistency reconciler repair dependencies are not configured")
	}
	a, err := r.assessments.FindByID(ctx, assessment.NewID(assessmentID))
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("assessment %d not found", assessmentID)
	}
	if a.Status().IsInterpreted() {
		return nil
	}
	if r.reports != nil {
		exists, err := r.reports.ReportExists(ctx, assessmentID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("assessment %d has no durable report to finalize", assessmentID)
		}
	}
	execution, err := r.snapshotStore.Load(ctx, assessmentID)
	if err != nil {
		return err
	}
	if execution == nil {
		return fmt.Errorf("assessment %d has no scoring snapshot for interpreted finalization", assessmentID)
	}
	if err := a.ApplyOutcome(execution); err != nil {
		return err
	}
	return r.assessmentSaver.Save(ctx, a)
}
