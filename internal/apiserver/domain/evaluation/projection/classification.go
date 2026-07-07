package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"

// ClassificationProjection enriches a scored outcome with typology decision semantics
// (type code, trait profile, match percent, etc.).
type ClassificationProjection interface {
	Apply(outcome *assessment.AssessmentOutcome) (*assessment.AssessmentOutcome, error)
}

// IdentityClassificationProjection is a pass-through for tests and no-op wiring.
type IdentityClassificationProjection struct{}

func (IdentityClassificationProjection) Apply(outcome *assessment.AssessmentOutcome) (*assessment.AssessmentOutcome, error) {
	return outcome, nil
}
