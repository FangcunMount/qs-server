// Package operator contains Evaluation use cases initiated by a background operator.
package operator

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
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
	authorizer  authorizer
}

func NewBatchExecutionService(assessments assessment.Repository, engine ExecutionEngine, access AccessChecker) BatchExecutionService {
	return &service{assessments: assessments, engine: engine, authorizer: authorizer{assessments: assessments, access: access}}
}

func (s *service) EvaluateBatch(ctx context.Context, actor Actor, assessmentIDs []uint64) (*BatchResult, error) {
	if err := s.authorizer.validateActor(actor); err != nil {
		return nil, err
	}
	if s.engine == nil {
		return nil, apperrors.ModuleNotConfigured("evaluation execution engine is not configured")
	}
	for _, id := range assessmentIDs {
		if _, err := s.authorizer.loadAssessment(ctx, actor, id); err != nil {
			return nil, err
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
