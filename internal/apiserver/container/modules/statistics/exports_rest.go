package statistics

import (
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

// RESTExportOptions carries container integration inputs for REST export.
type RESTExportOptions struct {
	TesteeAccessService          actorAccessApp.TesteeAccessService
	WarmupCoordinator            statisticsApp.WarmupCoordinator
	CacheGovernanceStatusService statisticsApp.GovernanceStatusReader
}

// ExportRESTDeps exposes statistics capabilities to REST transport.
func (m *Module) ExportRESTDeps(opts RESTExportOptions) resttransport.StatisticsDeps {
	deps := resttransport.StatisticsDeps{}
	if m == nil {
		return deps
	}
	deps.Enabled = true
	deps.ReadService = m.ReadService
	deps.PeriodicStatsService = m.PeriodicStatsService
	deps.SyncService = m.SyncService
	deps.TesteeAccessService = opts.TesteeAccessService
	deps.WarmupCoordinator = opts.WarmupCoordinator
	deps.CacheGovernanceStatusService = opts.CacheGovernanceStatusService
	deps.V2ReadService = m.V2ReadService
	deps.V2Coordinator = m.V2Coordinator
	deps.V2RunStore = m.V2RunStore
	return deps
}
