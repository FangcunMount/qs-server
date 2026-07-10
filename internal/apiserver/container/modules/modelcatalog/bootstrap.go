package modelcatalog

// BootstrapInput carries container integration inputs for assessment-model bootstrap.
type BootstrapInput struct {
	HotRank   HotRankDeps
	Lifecycle LifecycleDeps
	Typology  TypologyDeps
}

// Bootstrap assembles the unified catalog read model and family strategies.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps{
		HotRank:   in.HotRank,
		Lifecycle: in.Lifecycle,
		Typology:  in.Typology,
		TaskPerformance: TaskPerformanceDeps{
			ModelRepo:     in.Typology.ModelRepo,
			PublishedRepo: in.Typology.PublishedRepo,
			NormRepo:      in.Typology.NormRepo,
		},
		Norming: NormingDeps{
			ModelRepo:     in.Typology.ModelRepo,
			PublishedRepo: in.Typology.PublishedRepo,
			NormRepo:      in.Typology.NormRepo,
		},
	})
}
