package actor

import (
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

// ExportRESTDeps exposes actor capabilities to REST transport.
func (m *Module) ExportRESTDeps(qrCodeService qrcodeApp.QRCodeService) resttransport.ActorDeps {
	deps := resttransport.ActorDeps{}
	if m == nil {
		return deps
	}
	deps.TesteeManagementService = m.TesteeManagementService
	deps.TesteeQueryService = m.TesteeQueryService
	deps.TesteeBackendQueryService = m.TesteeBackendQueryService
	deps.TesteeAccessService = m.TesteeAccessService
	deps.OperatorLifecycleService = m.OperatorLifecycleService
	deps.OperatorAuthorizationService = m.OperatorAuthorizationService
	deps.OperatorQueryService = m.OperatorQueryService
	deps.ClinicianLifecycleService = m.ClinicianLifecycleService
	deps.ClinicianQueryService = m.ClinicianQueryService
	deps.ClinicianRelationshipService = m.ClinicianRelationshipService
	deps.AssessmentEntryService = m.AssessmentEntryService
	deps.QRCodeService = qrCodeService
	deps.ActiveOperatorChecker = m.ActiveOperatorChecker
	deps.OperatorRoleProjectionUpdater = m.OperatorRoleProjectionUpdater
	return deps
}
