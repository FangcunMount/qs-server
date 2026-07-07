package reporting

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type (
	ScaleReportBuilder             = FactorScoringReportBuilder
	ScaleScoreProjector            = FactorScoringScoreProjector
	BehavioralRatingReportBuilder  = NormProfileReportBuilder
	BehavioralRatingScoreProjector = NormProfileScoreProjector
	CognitiveReportBuilder         = TaskPerformanceReportBuilder
	CognitiveScoreProjector        = TaskPerformanceScoreProjector
)

// Deprecated: use NewFactorScoringReportBuilder.
func NewScaleReportBuilder(composer domainReport.ReportBuilder) ScaleReportBuilder {
	return NewFactorScoringReportBuilder(composer)
}

// Deprecated: use NewFactorScoringScoreProjector.
func NewScaleScoreProjector(scoreRepo assessment.ScoreRepository) ScaleScoreProjector {
	return NewFactorScoringScoreProjector(scoreRepo)
}

// Deprecated: use NewNormProfileReportBuilder.
func NewBehavioralRatingReportBuilder(composer domainReport.ReportBuilder) BehavioralRatingReportBuilder {
	return NewNormProfileReportBuilder(composer)
}

// Deprecated: use NewNormProfileScoreProjector.
func NewBehavioralRatingScoreProjector(scoreRepo assessment.ScoreRepository) NormProfileScoreProjector {
	return NewNormProfileScoreProjector(scoreRepo)
}

// Deprecated: use NewTaskPerformanceReportBuilder.
func NewCognitiveReportBuilder(composer domainReport.ReportBuilder) CognitiveReportBuilder {
	return NewTaskPerformanceReportBuilder(composer)
}

// Deprecated: use NewTaskPerformanceScoreProjector.
func NewCognitiveScoreProjector(scoreRepo assessment.ScoreRepository) CognitiveScoreProjector {
	return NewTaskPerformanceScoreProjector(scoreRepo)
}
