package evaluation

import domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"

// OutcomeRepository exposes the canonical Evaluation fact reader at the
// composition root. Interpretation consumes it without owning Evaluation.
func (m *Module) OutcomeRepository() domainoutcome.Repository {
	if m == nil {
		return nil
	}
	return m.outcomeRepository
}
