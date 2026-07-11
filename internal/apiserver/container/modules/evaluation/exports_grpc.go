package evaluation

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// ExportGRPCDeps exposes evaluation capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() grpctransport.EvaluationDeps {
	deps := grpctransport.EvaluationDeps{}
	if m == nil {
		return deps
	}
	deps.IntakeService = m.IntakeService
	deps.TesteeQueryService = m.TesteeQueryService
	deps.WorkerResultReader = m.WorkerResultReader
	deps.ScoreQueryService = m.ScoreQueryService
	deps.AssessmentReader = m.AssessmentReader
	deps.WorkerExecutionService = m.WorkerExecutionService
	deps.RunQueryService = m.RunQueryService
	deps.ReportStatusReporter = m.ReportStatusReporter
	return deps
}
