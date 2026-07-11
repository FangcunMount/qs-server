package interpretation

import (
	"context"
	"fmt"

	interpretationgeneration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/generation"
	interpretationinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/input"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// OutcomeReportService is the Interpretation application boundary consumed by
// worker/gRPC. It accepts an immutable EvaluationOutcome reference and returns
// Generation/Artifact state; it never exposes legacy InterpretReport writes.
type OutcomeReportService interface {
	GenerateByOutcomeID(ctx context.Context, outcomeID domainoutcome.ID) (*interpretationgeneration.ExecuteResult, error)
	GenerateByAssessmentID(ctx context.Context, assessmentID meta.ID) (*interpretationgeneration.ExecuteResult, error)
}

type outcomeReportService struct {
	outcomes domainoutcome.Repository
	executor interpretationgeneration.Executor
}

func NewOutcomeReportService(outcomes domainoutcome.Repository, executor interpretationgeneration.Executor) OutcomeReportService {
	return &outcomeReportService{outcomes: outcomes, executor: executor}
}

func (s *outcomeReportService) GenerateByAssessmentID(ctx context.Context, assessmentID meta.ID) (*interpretationgeneration.ExecuteResult, error) {
	if s == nil || s.outcomes == nil {
		return nil, fmt.Errorf("evaluation outcome repository is not configured")
	}
	record, err := s.outcomes.FindByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	return s.generate(ctx, record)
}

func (s *outcomeReportService) GenerateByOutcomeID(ctx context.Context, outcomeID domainoutcome.ID) (*interpretationgeneration.ExecuteResult, error) {
	if s == nil || s.outcomes == nil {
		return nil, fmt.Errorf("evaluation outcome repository is not configured")
	}
	if outcomeID.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	record, err := s.outcomes.FindByID(ctx, outcomeID)
	if err != nil {
		return nil, err
	}
	return s.generate(ctx, record)
}

func (s *outcomeReportService) generate(ctx context.Context, record *domainoutcome.Record) (*interpretationgeneration.ExecuteResult, error) {
	if s.executor == nil {
		return nil, fmt.Errorf("interpretation generation executor is not configured")
	}
	input, err := interpretationinput.FromOutcomeRecord(record)
	if err != nil {
		return nil, err
	}
	return interpretationgeneration.ExecuteOutcome(ctx, s.executor, record, input, "")
}
