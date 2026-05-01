package container

// moduleGraph owns apiserver cross-module post-wiring.
//
// Constructor dependencies remain the preferred path. This graph exists for
// late-bound dependencies where init order or optional infrastructure would
// otherwise force module constructors into cycles.
type moduleGraph struct {
	container *Container
}

func newModuleGraph(c *Container) moduleGraph {
	return moduleGraph{container: c}
}

func (g moduleGraph) postWireCacheGovernanceDependencies() {
	c := g.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.Handler == nil {
		return
	}
	c.StatisticsModule.Handler.SetWarmupCoordinator(c.WarmupCoordinator())
	c.StatisticsModule.Handler.SetCacheGovernanceStatusService(c.CacheGovernanceStatusService())
}

func (g moduleGraph) postWireProtectedScopeDependencies() {
	// Protected-scope dependencies are now constructor dependencies for modules
	// initialized after Actor. The hook stays as an explicit phase marker.
}

func (g moduleGraph) postWireQRCodeService() {
	// QRCode dependencies are constructor dependencies for modules that need them.
}
