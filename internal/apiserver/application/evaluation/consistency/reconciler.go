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

// ScoringArtifactChecker 判断计分产物是否已落库（快照或分数）。
type ScoringArtifactChecker interface {
	HasScoringArtifact(ctx context.Context, assessmentID uint64) (bool, error)
}

// Reconciler 扫描并修复 scoring/reporting 跨库部分成功窗口。
type Reconciler struct {
	assessments     AssessmentStatusReader
	artifacts       ScoringArtifactChecker
	snapshotStore   outcomescoring.SnapshotStore
	assessmentSaver assessment.Repository
}

// NewReconciler 创建跨存储对账器。
func NewReconciler(
	assessments AssessmentStatusReader,
	artifacts ScoringArtifactChecker,
	snapshotStore outcomescoring.SnapshotStore,
	assessmentSaver assessment.Repository,
) *Reconciler {
	return &Reconciler{
		assessments:     assessments,
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

// RepairEvaluatedFinalization 幂等重放 scoring writer 末步：ApplyScoringOutcome + assessment Save。
func (r *Reconciler) RepairEvaluatedFinalization(ctx context.Context, assessmentID uint64) error {
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
	if a.Status().IsEvaluated() || a.Status().IsInterpreted() {
		return nil
	}
	if !a.Status().IsSubmitted() {
		return fmt.Errorf("assessment %d status %s cannot finalize evaluated", assessmentID, a.Status())
	}
	if r.artifacts != nil {
		hasArtifact, err := r.artifacts.HasScoringArtifact(ctx, assessmentID)
		if err != nil {
			return err
		}
		if !hasArtifact {
			return fmt.Errorf("assessment %d has no scoring artifact to finalize", assessmentID)
		}
	}
	execution, err := r.snapshotStore.Load(ctx, assessmentID)
	if err != nil {
		return err
	}
	if execution == nil {
		return fmt.Errorf("assessment %d has no scoring snapshot for evaluated finalization", assessmentID)
	}
	if err := a.ApplyScoringOutcome(execution); err != nil {
		return err
	}
	return r.assessmentSaver.Save(ctx, a)
}
