package statistics

import (
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

// RESTExportOptions carries container integration inputs for REST export.
type RESTExportOptions struct {
	TesteeAccessService          actorAccessApp.TesteeAccessService
	WarmupCoordinator            cachegov.Coordinator
	CacheGovernanceStatusService cachegov.StatusService
}

// ExportRESTDeps exposes statistics capabilities to REST transport.
func (m *Module) ExportRESTDeps(opts RESTExportOptions) resttransport.StatisticsDeps {
	deps := resttransport.StatisticsDeps{}
	if m == nil {
		return deps
	}
	deps.Enabled = true
	deps.SystemStatisticsService = m.SystemStatisticsService
	deps.QuestionnaireStatisticsService = m.QuestionnaireStatisticsService
	deps.TesteeStatisticsService = m.TesteeStatisticsService
	deps.PlanStatisticsService = m.PlanStatisticsService
	deps.ReadService = m.ReadService
	deps.PeriodicStatsService = m.PeriodicStatsService
	deps.SyncService = m.SyncService
	deps.TesteeAccessService = opts.TesteeAccessService
	deps.WarmupCoordinator = opts.WarmupCoordinator
	deps.CacheGovernanceStatusService = opts.CacheGovernanceStatusService
	return deps
}
