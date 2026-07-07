package reporting

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type NormProfileReportBuilder struct {
	scoring FactorScoringReportBuilder
}

func NewNormProfileReportBuilder(composer domainReport.ReportBuilder) NormProfileReportBuilder {
	return NormProfileReportBuilder{scoring: NewFactorScoringReportBuilder(composer)}
}

func (NormProfileReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityBehavioralRatingDefault
}

func (NormProfileReportBuilder) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityBehavioralRatingDefault
}

func (NormProfileReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b NormProfileReportBuilder) Build(ctx context.Context, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if b.scoring.composer == nil {
		return nil, evalerrors.ModuleNotConfigured("norm_profile report builder is not configured")
	}
	return b.scoring.Build(ctx, outcome)
}

type NormProfileScoreProjector struct {
	scoring FactorScoringScoreProjector
}

func NewNormProfileScoreProjector(scoreRepo assessment.ScoreRepository) NormProfileScoreProjector {
	return NormProfileScoreProjector{scoring: NewFactorScoringScoreProjector(scoreRepo)}
}

func (NormProfileScoreProjector) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityBehavioralRatingDefault
}

func (NormProfileScoreProjector) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityBehavioralRatingDefault
}

func (p NormProfileScoreProjector) Project(ctx context.Context, outcome evaloutcome.Outcome) error {
	return p.scoring.Project(ctx, outcome)
}

type TaskPerformanceReportBuilder struct {
	scoring FactorScoringReportBuilder
}

func NewTaskPerformanceReportBuilder(composer domainReport.ReportBuilder) TaskPerformanceReportBuilder {
	return TaskPerformanceReportBuilder{scoring: NewFactorScoringReportBuilder(composer)}
}

func (TaskPerformanceReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityCognitiveDefault
}

func (TaskPerformanceReportBuilder) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityCognitiveDefault
}

func (TaskPerformanceReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b TaskPerformanceReportBuilder) Build(ctx context.Context, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if b.scoring.composer == nil {
		return nil, evalerrors.ModuleNotConfigured("task_performance report builder is not configured")
	}
	return b.scoring.Build(ctx, outcome)
}

type TaskPerformanceScoreProjector struct {
	scoring FactorScoringScoreProjector
}

func NewTaskPerformanceScoreProjector(scoreRepo assessment.ScoreRepository) TaskPerformanceScoreProjector {
	return TaskPerformanceScoreProjector{scoring: NewFactorScoringScoreProjector(scoreRepo)}
}

func (TaskPerformanceScoreProjector) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityCognitiveDefault
}

func (TaskPerformanceScoreProjector) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityCognitiveDefault
}

func (p TaskPerformanceScoreProjector) Project(ctx context.Context, outcome evaloutcome.Outcome) error {
	return p.scoring.Project(ctx, outcome)
}
