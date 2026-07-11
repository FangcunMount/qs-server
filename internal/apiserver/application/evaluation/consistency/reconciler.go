package consistency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// MismatchKind 标识跨存储不一致类型。
type MismatchKind string

const (
	MismatchOutcomeWithoutEvaluatedStatus MismatchKind = "outcome_without_evaluated_status"
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

// OutcomeChecker determines whether the canonical scoring fact exists.
// Assessment score rows are projections and must not be used for this decision.
type OutcomeChecker interface {
	HasOutcome(ctx context.Context, assessmentID uint64) (bool, error)
}

// Reconciler 扫描并修复 scoring/reporting 跨库部分成功窗口。
type Reconciler struct {
	assessments     AssessmentStatusReader
	outcomes        OutcomeChecker
	outcomeRepo     domainoutcome.Repository
	assessmentSaver assessment.Repository
}

// NewReconciler 创建跨存储对账器。
func NewReconciler(
	assessments AssessmentStatusReader,
	outcomes OutcomeChecker,
	outcomeRepo domainoutcome.Repository,
	assessmentSaver assessment.Repository,
) *Reconciler {
	return &Reconciler{
		assessments:     assessments,
		outcomes:        outcomes,
		outcomeRepo:     outcomeRepo,
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
	if r.outcomes != nil && a.Status().IsSubmitted() {
		hasOutcome, err := r.outcomes.HasOutcome(ctx, assessmentID)
		if err != nil {
			return nil, err
		}
		if hasOutcome {
			mismatches = append(mismatches, Mismatch{
				AssessmentID: assessmentID,
				Kind:         MismatchOutcomeWithoutEvaluatedStatus,
				DetectedAt:   detectedAt,
			})
		}
	}
	return mismatches, nil
}

// RepairEvaluatedFinalization 从持久化 EvaluationOutcome 重放 Assessment 的 evaluated 投影。
func (r *Reconciler) RepairEvaluatedFinalization(ctx context.Context, assessmentID uint64) error {
	if r == nil || r.assessments == nil || r.assessmentSaver == nil || r.outcomeRepo == nil {
		return fmt.Errorf("consistency reconciler repair dependencies are not configured")
	}
	a, err := r.assessments.FindByID(ctx, assessment.NewID(assessmentID))
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("assessment %d not found", assessmentID)
	}
	if a.Status().IsEvaluated() {
		return nil
	}
	if !a.Status().IsSubmitted() {
		return fmt.Errorf("assessment %d status %s cannot finalize evaluated", assessmentID, a.Status())
	}
	if r.outcomes != nil {
		hasOutcome, err := r.outcomes.HasOutcome(ctx, assessmentID)
		if err != nil {
			return err
		}
		if !hasOutcome {
			return fmt.Errorf("assessment %d has no evaluation outcome to finalize", assessmentID)
		}
	}
	record, err := r.outcomeRepo.FindByAssessmentID(ctx, assessment.NewID(assessmentID))
	if err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("assessment %d has no evaluation outcome for evaluated finalization", assessmentID)
	}
	var execution assessment.AssessmentOutcome
	if err := json.Unmarshal(record.Payload(), &execution); err != nil {
		return fmt.Errorf("decode evaluation outcome for assessment %d: %w", assessmentID, err)
	}
	if err := a.ApplyScoringOutcomeAt(&execution, record.EvaluatedAt()); err != nil {
		return err
	}
	return r.assessmentSaver.Save(ctx, a)
}
