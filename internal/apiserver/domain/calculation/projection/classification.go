package projection

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"

// ClassificationProjection enriches a scored result with typology decision semantics
// (type code, trait profile, match percent, etc.).
type ClassificationProjection interface {
	Apply(result *calculation.Result) (*calculation.Result, error)
}

// IdentityClassificationProjection is a pass-through for tests and no-op wiring.
type IdentityClassificationProjection struct{}

func (IdentityClassificationProjection) Apply(result *calculation.Result) (*calculation.Result, error) {
	return result, nil
}
