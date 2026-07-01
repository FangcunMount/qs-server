package scale

// CatalogCache 量表目录 REST DTO 进程内 L1 缓存。
type CatalogCache interface {
	GetDetail(code string) (*ScaleResponse, bool)
	SetDetail(code string, value *ScaleResponse)
	GetList(key string) (*ListScalesResponse, bool)
	SetList(key string, value *ListScalesResponse)
	GetListByRequest(req *ListScalesRequest) (*ListScalesResponse, bool)
	SetListByRequest(req *ListScalesRequest, value *ListScalesResponse)
	GetHotByRequest(req *ListHotScalesRequest) (*ListHotScalesResponse, bool)
	SetHotByRequest(req *ListHotScalesRequest, value *ListHotScalesResponse)
	GetCategories() (*ScaleCategoriesResponse, bool)
	SetCategories(value *ScaleCategoriesResponse)
	GetHot(key string) (*ListHotScalesResponse, bool)
	SetHot(key string, value *ListHotScalesResponse)
	EvictOnSignal(code string)
	Stats() (hits, misses uint64)
}

const defaultCatalogCacheTTLSeconds = 180
