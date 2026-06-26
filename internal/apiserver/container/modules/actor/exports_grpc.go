package actor

import grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"

// ExportGRPCDeps exposes actor capabilities to gRPC transport.
func (m *Module) ExportGRPCDeps() grpctransport.ActorDeps {
	deps := grpctransport.ActorDeps{}
	if m == nil {
		return deps
	}
	deps.TesteeRegistrationService = m.TesteeRegistrationService
	deps.TesteeManagementService = m.TesteeManagementService
	deps.TesteeQueryService = m.TesteeQueryService
	deps.ClinicianRelationshipService = m.ClinicianRelationshipService
	deps.TesteeAssessmentAttentionService = m.TesteeAssessmentAttentionService
	deps.OperatorLifecycleService = m.OperatorLifecycleService
	deps.OperatorAuthorizationService = m.OperatorAuthorizationService
	deps.OperatorQueryService = m.OperatorQueryService
	deps.OperatorRoleProjectionUpdater = m.OperatorRoleProjectionUpdater
	return deps
}
