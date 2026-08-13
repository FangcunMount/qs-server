package modelcatalog

// PublishedModelCache owns collection-server's final published-model REST DTO
// cache. It deliberately excludes hot-rank responses.
type PublishedModelCache interface {
	GetDetail(code string) (*ModelResponse, bool)
	SetDetail(code string, value *ModelResponse)
	GetListByRequest(request *ListRequest) (*ListResponse, bool)
	SetListByRequest(request *ListRequest, value *ListResponse)
	GetOptions(kind string) (*OptionsResponse, bool)
	SetOptions(kind string, value *OptionsResponse)
	EvictOnSignal(code string)
}
