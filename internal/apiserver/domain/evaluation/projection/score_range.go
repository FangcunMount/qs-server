package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"

// ScoreRangeProjection is a no-op projection for factor_scoring models whose
// interpretation is already embedded in scale scoring.
type ScoreRangeProjection struct{}

func (ScoreRangeProjection) Apply(outcome *assessment.AssessmentOutcome) *assessment.AssessmentOutcome {
	return outcome
}
