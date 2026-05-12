package evaluation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	scaleevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Service interface {
	Evaluate(
		ctx context.Context,
		a *assessment.Assessment,
		snapshot *evaluationinput.InputSnapshot,
	) (*assessment.EvaluationResult, error)
}

type scaleEvaluationService struct {
	validator InputValidator
	assembler InputAssembler
	evaluator *scaleevaluation.Evaluator
	mapper    ResultMapper
}

func NewService(
	validator InputValidator,
	assembler InputAssembler,
	evaluator *scaleevaluation.Evaluator,
	mapper ResultMapper,
) Service {
	if validator == nil {
		validator = DefaultInputValidator{}
	}
	if assembler == nil {
		assembler = DefaultInputAssembler{}
	}
	if evaluator == nil {
		evaluator = scaleevaluation.NewDefaultEvaluator()
	}
	if mapper == nil {
		mapper = DefaultResultMapper{}
	}
	return &scaleEvaluationService{
		validator: validator,
		assembler: assembler,
		evaluator: evaluator,
		mapper:    mapper,
	}
}

func (s *scaleEvaluationService) Evaluate(
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
	scaleInput := s.assembler.FromSnapshot(snapshot)
	result, err := s.evaluator.Evaluate(ctx, scaleInput)
	if err != nil {
		return nil, err
	}
	return s.mapper.ToEvaluationResult(result, a, snapshot), nil
}
