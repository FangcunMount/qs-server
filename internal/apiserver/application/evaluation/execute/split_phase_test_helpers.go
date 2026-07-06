package execute

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evaluationscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scoring"
	interpretationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// splitPhaseCapture records split-phase persistence for execute package tests.
type splitPhaseCapture struct {
	ScoringCalls        int
	InterpretationCalls int
	Outcome             evaloutcome.Outcome
}

type recordingSplitPhaseScoringWriter struct {
	capture *splitPhaseCapture
}

func (w *recordingSplitPhaseScoringWriter) Write(_ context.Context, outcome evaloutcome.Outcome) error {
	w.capture.ScoringCalls++
	if outcome.Assessment != nil && outcome.Execution != nil {
		return outcome.Assessment.ApplyScoringOutcome(outcome.Execution)
	}
	return nil
}

type recordingSplitPhaseInterpretationService struct {
	capture *splitPhaseCapture
}

func (s *recordingSplitPhaseInterpretationService) GenerateAndPersist(_ context.Context, outcome evaloutcome.Outcome) error {
	s.capture.InterpretationCalls++
	s.capture.Outcome = outcome
	return nil
}

func newSplitPhaseTestService(
	repo assessment.Repository,
	input evaluationinput.Resolver,
	capture *splitPhaseCapture,
	opts ...ServiceOption,
) Service {
	base := []ServiceOption{
		WithScoringWriter(&recordingSplitPhaseScoringWriter{capture: capture}),
		WithInterpretationService(&recordingSplitPhaseInterpretationService{capture: capture}),
	}
	return NewService(repo, input, append(base, opts...)...)
}

var (
	_ evaluationscoring.Writer  = (*recordingSplitPhaseScoringWriter)(nil)
	_ interpretationapp.Service = (*recordingSplitPhaseInterpretationService)(nil)
)
