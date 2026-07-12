package evaluation

import "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"

// OutcomeRepository exposes the canonical Evaluation fact reader at the
// composition root. Interpretation consumes it without owning Evaluation.
func (m *Module) OutcomeRepository() evaluationfact.Repository {
	if m == nil {
		return nil
	}
	return newEvaluationFactRepository(m.outcomeRepository)
}
