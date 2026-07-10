package modelcatalog

// BootstrapInput carries container integration inputs for assessment-model bootstrap.
type BootstrapInput struct {
	HotRank   HotRankDeps
	Lifecycle LifecycleDeps
	Catalog   CatalogDeps
}

// Bootstrap assembles the unified catalog read model and family strategies.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps{
		HotRank:   in.HotRank,
		Lifecycle: in.Lifecycle,
		Catalog:   in.Catalog,
	})
}
