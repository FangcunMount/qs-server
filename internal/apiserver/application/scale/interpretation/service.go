package interpretation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/interpretation"
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
	assembler InputAssembler
	evaluator *scaleinterpretation.Evaluator
	mapper    ResultMapper
}

func NewService(
	validator InputValidator,
	assembler InputAssembler,
	evaluator *scaleinterpretation.Evaluator,
	mapper ResultMapper,
) Service {
	if validator == nil {
		validator = DefaultInputValidator{}
	}
	if assembler == nil {
		assembler = DefaultInputAssembler{}
	}
	if evaluator == nil {
		evaluator = scaleinterpretation.NewDefaultEvaluator()
	}
	if mapper == nil {
		mapper = DefaultResultMapper{}
	}
	return &scaleInterpretationService{
		validator: validator,
		assembler: assembler,
		evaluator: evaluator,
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
	scaleInput := s.assembler.FromSnapshot(snapshot)
	result, err := s.evaluator.Evaluate(ctx, scaleInput)
	if err != nil {
		return nil, err
	}
	return s.mapper.ToEvaluationResult(result, a, snapshot), nil
}
