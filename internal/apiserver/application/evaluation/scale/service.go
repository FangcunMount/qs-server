package scale

import (
	"context"

	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Service interface {
	Evaluate(
		ctx context.Context,
		a *assessment.Assessment,
		snapshot *evaluationinput.InputSnapshot,
	) (*assessment.EvaluationResult, error)
}

type scaleInterpretationService struct {
	validator InputValidator
	handler   *evaluationdomain.ScaleHandler
	mapper    ResultMapper
}

func NewService(
	validator InputValidator,
	handler *evaluationdomain.ScaleHandler,
	mapper ResultMapper,
) Service {
	if validator == nil {
		validator = DefaultInputValidator{}
	}
	if handler == nil {
		handler = evaluationdomain.NewDefaultScaleHandler()
	}
	if mapper == nil {
		mapper = DefaultResultMapper{}
	}
	return &scaleInterpretationService{
		validator: validator,
		handler:   handler,
		mapper:    mapper,
	}
}

func (s *scaleInterpretationService) Evaluate(
	ctx context.Context,
	a *assessment.Assessment,
	snapshot *evaluationinput.InputSnapshot,
) (*assessment.EvaluationResult, error) {
	input := ScaleExecutionInput{
		Assessment: a,
		Input:      snapshot,
	}
	if err := s.validator.Validate(input); err != nil {
		return nil, err
	}
	result, err := s.handler.Evaluate(ctx, scaleEvaluateInputFromSnapshot(snapshot))
	if err != nil {
		return nil, err
	}
	return s.mapper.ToEvaluationResult(result, a, snapshot), nil
}
