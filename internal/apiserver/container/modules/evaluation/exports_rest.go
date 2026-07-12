package evaluation

import (
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
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
	deps.RunQueryService = m.RunQueryService
	return deps
}

// ExportTesteeScaleAnalysisService composes actor-facing analysis from evaluation query ports.
func (m *Module) ExportTesteeScaleAnalysisService() testeeApp.ScaleAnalysisQueryService {
	if m == nil || m.OperatorQueryService == nil || m.ScoreQueryService == nil {
		return nil
	}
	return testeeApp.NewScaleAnalysisQueryService(m.OperatorQueryService, m.ScoreQueryService)
}

// ExportRESTEventStatusOutbox exposes the assessment outbox status reader for platform event status.
func (m *Module) ExportRESTEventStatusOutbox() appEventing.NamedOutboxStatusReader {
	if m == nil {
		return appEventing.NamedOutboxStatusReader{}
	}
	return m.AssessmentOutboxStatusReader
}
