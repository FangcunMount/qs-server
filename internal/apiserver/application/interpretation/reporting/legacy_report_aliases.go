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

// Deprecated: 使用 NewFactorScoringReportBuilder。
func NewScaleReportBuilder(composer domainReport.ReportBuilder) ScaleReportBuilder {
	return NewFactorScoringReportBuilder(composer)
}

// Deprecated: 使用 NewFactorScoringScoreProjector。
func NewScaleScoreProjector(scoreRepo assessment.ScoreRepository) ScaleScoreProjector {
	return NewFactorScoringScoreProjector(scoreRepo)
}

// Deprecated: 使用 NewNormProfileReportBuilder。
func NewBehavioralRatingReportBuilder(composer domainReport.ReportBuilder) BehavioralRatingReportBuilder {
	return NewNormProfileReportBuilder(composer)
}

// Deprecated: 使用 NewNormProfileScoreProjector。
func NewBehavioralRatingScoreProjector(scoreRepo assessment.ScoreRepository) NormProfileScoreProjector {
	return NewNormProfileScoreProjector(scoreRepo)
}

// Deprecated: 使用 NewTaskPerformanceReportBuilder。
func NewCognitiveReportBuilder(composer domainReport.ReportBuilder) CognitiveReportBuilder {
	return NewTaskPerformanceReportBuilder(composer)
}

// Deprecated: 使用 NewTaskPerformanceScoreProjector。
func NewCognitiveScoreProjector(scoreRepo assessment.ScoreRepository) CognitiveScoreProjector {
	return NewTaskPerformanceScoreProjector(scoreRepo)
}
