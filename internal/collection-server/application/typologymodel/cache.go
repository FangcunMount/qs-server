package typologymodel

// CatalogCache 人格模型目录 REST DTO 进程内 L1 缓存。
type CatalogCache interface {
	GetDetail(code string) (*TypologyModelResponse, bool)
	SetDetail(code string, value *TypologyModelResponse)
	GetListByRequest(req *ListTypologyModelsRequest) (*ListTypologyModelsResponse, bool)
	SetListByRequest(req *ListTypologyModelsRequest, value *ListTypologyModelsResponse)
	GetCategories() (*TypologyModelCategoriesResponse, bool)
	SetCategories(value *TypologyModelCategoriesResponse)
	EvictOnSignal(code string)
	Stats() (hits, misses uint64)
}
