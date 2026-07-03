package personalitymodel

// CatalogCache 人格模型目录 REST DTO 进程内 L1 缓存。
type CatalogCache interface {
	GetDetail(code string) (*PersonalityModelResponse, bool)
	SetDetail(code string, value *PersonalityModelResponse)
	GetListByRequest(req *ListPersonalityModelsRequest) (*ListPersonalityModelsResponse, bool)
	SetListByRequest(req *ListPersonalityModelsRequest, value *ListPersonalityModelsResponse)
	GetCategories() (*PersonalityModelCategoriesResponse, bool)
	SetCategories(value *PersonalityModelCategoriesResponse)
	EvictOnSignal(code string)
	Stats() (hits, misses uint64)
}
