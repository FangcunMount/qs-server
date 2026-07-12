package evaluation

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// ExportGRPCDeps exposes evaluation capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() grpctransport.EvaluationDeps {
	deps := grpctransport.EvaluationDeps{}
	if m == nil {
		return deps
	}
	deps.IntakeService = m.IntakeService
	deps.TesteeService = m.TesteeService
	deps.WorkerService = m.WorkerService
	return deps
}
