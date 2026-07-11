package evaluation

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// ExportGRPCDeps exposes evaluation capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() grpctransport.EvaluationDeps {
	deps := grpctransport.EvaluationDeps{}
	if m == nil {
		return deps
	}
	deps.SubmissionService = m.SubmissionService
	deps.ManagementService = m.ManagementService
	deps.ReportQueryService = m.ReportQueryService
	deps.ScoreQueryService = m.ScoreQueryService
	deps.AssessmentReader = m.AssessmentReader
	deps.EvaluationService = m.EvaluationService
	deps.RunQueryService = m.RunQueryService
	deps.ReportStatusReporter = m.ReportStatusReporter
	return deps
}
