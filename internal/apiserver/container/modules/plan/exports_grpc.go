package plan

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// ExportGRPCDeps exposes plan capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() grpctransport.PlanDeps {
	deps := grpctransport.PlanDeps{}
	if m == nil {
		return deps
	}
	deps.CommandService = m.CommandService
	deps.TaskAssessmentResolver = m.TaskAssessmentResolver
	return deps
}
