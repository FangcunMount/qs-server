package modelcatalog

// BootstrapInput 模型目录的启动输入
type BootstrapInput struct {
	HotRank   HotRankDeps
	Lifecycle LifecycleDeps
	Catalog   CatalogDeps
}

// Bootstrap 模型目录的启动函数
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps{
		HotRank:   in.HotRank,
		Lifecycle: in.Lifecycle,
		Catalog:   in.Catalog,
	})
}
