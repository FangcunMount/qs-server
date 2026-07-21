package plan

import (
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

// ExportRESTDeps exposes plan capabilities to REST transport.
func (m *Module) ExportRESTDeps(testeeAccess actorAccessApp.TesteeAccessService) resttransport.PlanDeps {
	deps := resttransport.PlanDeps{}
	if m == nil {
		return deps
	}
	deps.CommandService = m.CommandService
	deps.QueryService = m.QueryService
	deps.EnrollmentQueryService = m.EnrollmentQueryService
	deps.TesteeAccessService = testeeAccess
	return deps
}
