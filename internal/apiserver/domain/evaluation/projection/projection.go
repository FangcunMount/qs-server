package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"

// OutcomeProjection enriches a raw factor-scoring outcome with algorithm-family semantics.
type OutcomeProjection interface {
	Apply(outcome *assessment.AssessmentOutcome) *assessment.AssessmentOutcome
}
