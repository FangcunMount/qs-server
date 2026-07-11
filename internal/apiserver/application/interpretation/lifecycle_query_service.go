package interpretation

import (
	"context"
	"fmt"

	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// LifecycleQueryService exposes the three-object Interpretation read model.
// Assessment lookup is a correlation query through Outcome, never a ReportID
// alias; a single assessment can have multiple historical template artifacts.
type LifecycleQueryService interface {
	FindReportByID(ctx context.Context, reportID meta.ID) (*domainreport.Artifact, error)
	FindGenerationsByOutcomeID(ctx context.Context, outcomeID meta.ID) ([]GenerationResult, error)
	FindLatestReportByAssessmentID(ctx context.Context, assessmentID meta.ID) (*domainreport.Artifact, error)
	FindLifecycleByAssessmentID(ctx context.Context, assessmentID meta.ID) ([]GenerationResult, error)
	ListHistoricalReportsByAssessmentID(ctx context.Context, assessmentID meta.ID) ([]*domainreport.Artifact, error)
}

// OutcomeCorrelationPort resolves AssessmentID -> OutcomeID without importing
// Evaluation domain types into this package.
type OutcomeCorrelationPort interface {
	FindOutcomeIDByAssessmentID(ctx context.Context, assessmentID meta.ID) (meta.ID, error)
}

type GenerationResult struct {
	Generation *domaingeneration.ReportGeneration
	LatestRun  *interpretationrun.InterpretationRun
	Artifact   *domainreport.Artifact
}

type lifecycleQueryService struct {
	outcomes    OutcomeCorrelationPort
	generations domaingeneration.Repository
	runs        interpretationrun.Repository
	artifacts   domainreport.ArtifactRepository
}

func NewLifecycleQueryService(
	outcomes OutcomeCorrelationPort,
	generations domaingeneration.Repository,
	runs interpretationrun.Repository,
	artifacts domainreport.ArtifactRepository,
) LifecycleQueryService {
	return &lifecycleQueryService{outcomes: outcomes, generations: generations, runs: runs, artifacts: artifacts}
}

func (s *lifecycleQueryService) FindReportByID(ctx context.Context, reportID meta.ID) (*domainreport.Artifact, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s.artifacts.FindByID(ctx, reportID)
}

func (s *lifecycleQueryService) FindGenerationsByOutcomeID(ctx context.Context, outcomeID meta.ID) ([]GenerationResult, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if outcomeID.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	items, err := s.generations.ListByOutcomeID(ctx, outcomeID)
	if err != nil {
		return nil, err
	}
	return s.toResults(ctx, items)
}

func (s *lifecycleQueryService) FindLatestReportByAssessmentID(ctx context.Context, assessmentID meta.ID) (*domainreport.Artifact, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s.artifacts.FindLatestByAssessmentID(ctx, assessmentID)
}

func (s *lifecycleQueryService) FindLifecycleByAssessmentID(ctx context.Context, assessmentID meta.ID) ([]GenerationResult, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if assessmentID.IsZero() {
		return nil, fmt.Errorf("assessment id is required")
	}
	outcomeID, err := s.outcomes.FindOutcomeIDByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	return s.FindGenerationsByOutcomeID(ctx, outcomeID)
}

func (s *lifecycleQueryService) ListHistoricalReportsByAssessmentID(ctx context.Context, assessmentID meta.ID) ([]*domainreport.Artifact, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s.artifacts.ListByAssessmentID(ctx, assessmentID)
}

func (s *lifecycleQueryService) toResults(ctx context.Context, generations []*domaingeneration.ReportGeneration) ([]GenerationResult, error) {
	results := make([]GenerationResult, 0, len(generations))
	for _, generation := range generations {
		if generation == nil {
			continue
		}
		result := GenerationResult{Generation: generation}
		if !generation.LatestRunID().IsZero() {
			run, err := s.runs.FindByID(ctx, generation.LatestRunID())
			if err != nil {
				return nil, err
			}
			result.LatestRun = run
		}
		if generation.Status() == domaingeneration.StatusGenerated {
			artifact, err := s.artifacts.FindByGenerationID(ctx, generation.ID())
			if err != nil {
				return nil, err
			}
			result.Artifact = artifact
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *lifecycleQueryService) validate() error {
	if s == nil || s.outcomes == nil || s.generations == nil || s.runs == nil || s.artifacts == nil {
		return fmt.Errorf("interpretation lifecycle query service is not configured")
	}
	return nil
}
