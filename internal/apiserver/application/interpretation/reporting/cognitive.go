package reporting

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type CognitiveReportBuilder struct {
	scale ScaleReportBuilder
}

func NewCognitiveReportBuilder(composer domainReport.ReportBuilder) CognitiveReportBuilder {
	return CognitiveReportBuilder{scale: NewScaleReportBuilder(composer)}
}

func (CognitiveReportBuilder) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyCognitiveDefault
}

func (CognitiveReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b CognitiveReportBuilder) Build(ctx context.Context, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if b.scale.composer == nil {
		return nil, evalerrors.ModuleNotConfigured("cognitive report builder is not configured")
	}
	return b.scale.Build(ctx, outcome)
}

type CognitiveScoreProjector struct {
	scale ScaleScoreProjector
}

func NewCognitiveScoreProjector(scoreRepo assessment.ScoreRepository) CognitiveScoreProjector {
	return CognitiveScoreProjector{scale: NewScaleScoreProjector(scoreRepo)}
}

func (CognitiveScoreProjector) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyCognitiveDefault
}

func (p CognitiveScoreProjector) Project(ctx context.Context, outcome evaloutcome.Outcome) error {
	return p.scale.Project(ctx, outcome)
}
