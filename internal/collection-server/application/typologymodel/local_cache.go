package typologymodel

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
)

const (
	cacheKeyPrefixDetail = "personality:detail:"
	cacheKeyCategories   = "personality:categories"
	cacheKeyPrefixList   = "personality:list:"
)

// LocalCatalogCache 人格模型目录进程内 TTL 缓存。
type LocalCatalogCache struct {
	inner *catalogl1.MultiCache[*PersonalityModelResponse, *ListPersonalityModelsResponse, *PersonalityModelCategoriesResponse, struct{}]
}

// LocalCatalogCacheOptions 人格模型目录 L1 配置。
type LocalCatalogCacheOptions struct {
	TTL            time.Duration
	MaxEntries     int
	TTLJitterRatio float64
	OnHit          func()
	OnMiss         func()
}

// NewLocalCatalogCache 创建人格模型目录 L1 缓存。
func NewLocalCatalogCache(opts LocalCatalogCacheOptions) *LocalCatalogCache {
	return &LocalCatalogCache{
		inner: catalogl1.NewMultiCache(catalogl1.Options{
			TTL:            opts.TTL,
			MaxEntries:     opts.MaxEntries,
			TTLJitterRatio: opts.TTLJitterRatio,
			OnHit:          opts.OnHit,
			OnMiss:         opts.OnMiss,
		}, catalogl1.MultiHooks[*PersonalityModelResponse, *ListPersonalityModelsResponse, *PersonalityModelCategoriesResponse, struct{}]{
			DetailKey:       detailCacheKey,
			ListKey:         func(req any) string { return listCacheKey(req.(*ListPersonalityModelsRequest)) },
			CategoriesKey:   cacheKeyCategories,
			ListPrefix:      cacheKeyPrefixList,
			CloneDetail:     clonePersonalityModelResponse,
			CloneList:       cloneListPersonalityModelsResponse,
			CloneCategories: clonePersonalityModelCategoriesResponse,
			CloneHot:        nil,
		}),
	}
}

func detailCacheKey(code string) string {
	return cacheKeyPrefixDetail + strings.ToLower(strings.TrimSpace(code))
}

func listCacheKey(req *ListPersonalityModelsRequest) string {
	if req == nil {
		req = &ListPersonalityModelsRequest{}
	}
	return fmt.Sprintf("%sp%d:ps%d:a:%s", cacheKeyPrefixList, req.Page, req.PageSize, strings.ToLower(strings.TrimSpace(req.Algorithm)))
}

func (c *LocalCatalogCache) GetDetail(code string) (*PersonalityModelResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.GetDetail(code)
}

func (c *LocalCatalogCache) SetDetail(code string, value *PersonalityModelResponse) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.SetDetail(code, value)
}

func (c *LocalCatalogCache) GetListByRequest(req *ListPersonalityModelsRequest) (*ListPersonalityModelsResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	if req == nil {
		req = &ListPersonalityModelsRequest{}
	}
	return c.inner.GetListByRequest(req)
}

func (c *LocalCatalogCache) SetListByRequest(req *ListPersonalityModelsRequest, value *ListPersonalityModelsResponse) {
	if c == nil || c.inner == nil {
		return
	}
	if req == nil {
		req = &ListPersonalityModelsRequest{}
	}
	c.inner.SetListByRequest(req, value)
}

func (c *LocalCatalogCache) GetCategories() (*PersonalityModelCategoriesResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.GetCategories()
}

func (c *LocalCatalogCache) SetCategories(value *PersonalityModelCategoriesResponse) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.SetCategories(value)
}

func (c *LocalCatalogCache) EvictOnSignal(code string) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.EvictOnSignal(code)
}

func (c *LocalCatalogCache) Stats() (hits, misses uint64) {
	if c == nil || c.inner == nil {
		return 0, 0
	}
	return c.inner.Stats()
}
