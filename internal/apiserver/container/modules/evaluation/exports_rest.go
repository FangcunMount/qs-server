package evaluation

import (
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

// ExportRESTDeps exposes evaluation capabilities to REST transport.
func (m *Module) ExportRESTDeps() resttransport.EvaluationDeps {
	deps := resttransport.EvaluationDeps{}
	if m == nil {
		return deps
	}
	deps.OperatorRecoveryService = m.OperatorRecovery
	deps.OperatorExecutionService = m.OperatorExecutionService
	deps.ProtectedQueryService = m.OperatorQuery
	return deps
}

// ExportTesteeScaleAnalysisService composes actor-facing analysis from evaluation query ports.
func (m *Module) ExportTesteeScaleAnalysisService() evaluationoperator.ScaleAnalysisService {
	if m == nil {
		return nil
	}
	return m.ScaleAnalysis
}
