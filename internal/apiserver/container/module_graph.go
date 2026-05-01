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

func (g moduleGraph) postWireScaleDependencies() {
	c := g.container
	if c == nil || c.SurveyModule == nil || c.ScaleModule == nil {
		return
	}
	c.SurveyModule.SetScaleRepository(c.ScaleModule.Repo)
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
	c := g.container
	if c == nil || c.QRCodeService == nil {
		return
	}
	if c.EvaluationModule != nil {
		c.EvaluationModule.SetQRCodeService(c.QRCodeService)
	}
	if c.SurveyModule != nil {
		c.SurveyModule.SetQRCodeService(c.QRCodeService)
	}
	if c.ScaleModule != nil {
		c.ScaleModule.SetQRCodeService(c.QRCodeService)
	}
}
