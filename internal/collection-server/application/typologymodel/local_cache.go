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

// LocalCatalogCache 类型学模型目录进程内 TTL 缓存。
type LocalCatalogCache struct {
	inner *catalogl1.MultiCache[*TypologyModelResponse, *ListTypologyModelsResponse, *TypologyModelCategoriesResponse, struct{}]
}

// LocalCatalogCacheOptions 类型学模型目录 L1 配置。
type LocalCatalogCacheOptions struct {
	TTL            time.Duration
	MaxEntries     int
	TTLJitterRatio float64
	OnHit          func()
	OnMiss         func()
}

// NewLocalCatalogCache 创建类型学模型目录 L1 缓存。
func NewLocalCatalogCache(opts LocalCatalogCacheOptions) *LocalCatalogCache {
	return &LocalCatalogCache{
		inner: catalogl1.NewMultiCache(catalogl1.Options{
			TTL:            opts.TTL,
			MaxEntries:     opts.MaxEntries,
			TTLJitterRatio: opts.TTLJitterRatio,
			OnHit:          opts.OnHit,
			OnMiss:         opts.OnMiss,
		}, catalogl1.MultiHooks[*TypologyModelResponse, *ListTypologyModelsResponse, *TypologyModelCategoriesResponse, struct{}]{
			DetailKey:       detailCacheKey,
			ListKey:         func(req any) string { return listCacheKey(req.(*ListTypologyModelsRequest)) },
			CategoriesKey:   cacheKeyCategories,
			ListPrefix:      cacheKeyPrefixList,
			CloneDetail:     cloneTypologyModelResponse,
			CloneList:       cloneListTypologyModelsResponse,
			CloneCategories: cloneTypologyModelCategoriesResponse,
			CloneHot:        nil,
		}),
	}
}

func detailCacheKey(code string) string {
	return cacheKeyPrefixDetail + strings.ToLower(strings.TrimSpace(code))
}

func listCacheKey(req *ListTypologyModelsRequest) string {
	if req == nil {
		req = &ListTypologyModelsRequest{}
	}
	return fmt.Sprintf("%sp%d:ps%d", cacheKeyPrefixList, req.Page, req.PageSize)
}

func (c *LocalCatalogCache) GetDetail(code string) (*TypologyModelResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.GetDetail(code)
}

func (c *LocalCatalogCache) SetDetail(code string, value *TypologyModelResponse) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.SetDetail(code, value)
}

func (c *LocalCatalogCache) GetListByRequest(req *ListTypologyModelsRequest) (*ListTypologyModelsResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	if req == nil {
		req = &ListTypologyModelsRequest{}
	}
	return c.inner.GetListByRequest(req)
}

func (c *LocalCatalogCache) SetListByRequest(req *ListTypologyModelsRequest, value *ListTypologyModelsResponse) {
	if c == nil || c.inner == nil {
		return
	}
	if req == nil {
		req = &ListTypologyModelsRequest{}
	}
	c.inner.SetListByRequest(req, value)
}

func (c *LocalCatalogCache) GetCategories() (*TypologyModelCategoriesResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.GetCategories()
}

func (c *LocalCatalogCache) SetCategories(value *TypologyModelCategoriesResponse) {
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
