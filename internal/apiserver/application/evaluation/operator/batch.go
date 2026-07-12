// Package operator contains Evaluation use cases initiated by a background operator.
package operator

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Actor struct {
	OrgID          int64
	OperatorUserID int64
}

type BatchExecutionService interface {
	EvaluateBatch(ctx context.Context, actor Actor, assessmentIDs []uint64) (*BatchResult, error)
}

// ExecutionEngine is the actor-neutral mechanism used by operator batch
// execution. It is not the Worker actor application service.
type ExecutionEngine interface {
	Evaluate(context.Context, uint64) error
}

type BatchResult struct {
	TotalCount   int
	SuccessCount int
	FailedCount  int
	FailedIDs    []uint64
}

type service struct {
	assessments assessment.Repository
	engine      ExecutionEngine
}

func NewBatchExecutionService(assessments assessment.Repository, engine ExecutionEngine) BatchExecutionService {
	return &service{assessments: assessments, engine: engine}
}

func (s *service) EvaluateBatch(ctx context.Context, actor Actor, assessmentIDs []uint64) (*BatchResult, error) {
	if actor.OrgID == 0 {
		return nil, apperrors.InvalidArgument("机构ID不能为空")
	}
	for _, id := range assessmentIDs {
		a, err := s.assessments.FindByID(ctx, meta.FromUint64(id))
		if err != nil {
			return nil, apperrors.AssessmentNotFound(err, "测评不存在")
		}
		if a.OrgID() != actor.OrgID {
			return nil, apperrors.PermissionDenied("测评不属于当前机构")
		}
	}
	result := &BatchResult{TotalCount: len(assessmentIDs), FailedIDs: make([]uint64, 0)}
	for _, id := range assessmentIDs {
		if err := s.engine.Evaluate(ctx, id); err != nil {
			result.FailedCount++
			result.FailedIDs = append(result.FailedIDs, id)
			continue
		}
		result.SuccessCount++
	}
	return result, nil
}
