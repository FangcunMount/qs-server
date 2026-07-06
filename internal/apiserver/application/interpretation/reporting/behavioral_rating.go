package reporting

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type BehavioralRatingReportBuilder struct {
	scale ScaleReportBuilder
}

func NewBehavioralRatingReportBuilder(composer domainReport.ReportBuilder) BehavioralRatingReportBuilder {
	return BehavioralRatingReportBuilder{scale: NewScaleReportBuilder(composer)}
}

func (BehavioralRatingReportBuilder) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyBehavioralRatingDefault
}

func (BehavioralRatingReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b BehavioralRatingReportBuilder) Build(ctx context.Context, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if b.scale.composer == nil {
		return nil, evalerrors.ModuleNotConfigured("behavioral_rating report builder is not configured")
	}
	return b.scale.Build(ctx, outcome)
}

type BehavioralRatingScoreProjector struct {
	scale ScaleScoreProjector
}

func NewBehavioralRatingScoreProjector(scoreRepo assessment.ScoreRepository) BehavioralRatingScoreProjector {
	return BehavioralRatingScoreProjector{scale: NewScaleScoreProjector(scoreRepo)}
}

func (BehavioralRatingScoreProjector) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyBehavioralRatingDefault
}

func (p BehavioralRatingScoreProjector) Project(ctx context.Context, outcome evaloutcome.Outcome) error {
	return p.scale.Project(ctx, outcome)
}
