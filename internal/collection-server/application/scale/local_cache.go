package scale

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/catalogl1"
)

const (
	cacheKeyPrefixDetail = "scale:detail:"
	cacheKeyCategories   = "scale:categories"
	cacheKeyPrefixList   = "scale:list:"
	cacheKeyPrefixHot    = "scale:hot:"
)

// LocalCatalogCache 量表目录进程内 TTL 缓存。
type LocalCatalogCache struct {
	inner *catalogl1.MultiCache[*ScaleResponse, *ListScalesResponse, *ScaleCategoriesResponse, *ListHotScalesResponse]
}

// LocalCatalogCacheOptions 量表目录 L1 配置。
type LocalCatalogCacheOptions struct {
	TTL            time.Duration
	MaxEntries     int
	TTLJitterRatio float64
	OnHit          func()
	OnMiss         func()
}

// NewLocalCatalogCache 创建量表目录 L1 缓存。
func NewLocalCatalogCache(opts LocalCatalogCacheOptions) *LocalCatalogCache {
	return &LocalCatalogCache{
		inner: catalogl1.NewMultiCache(catalogl1.Options{
			TTL:            opts.TTL,
			MaxEntries:     opts.MaxEntries,
			TTLJitterRatio: opts.TTLJitterRatio,
			OnHit:          opts.OnHit,
			OnMiss:         opts.OnMiss,
		}, catalogl1.MultiHooks[*ScaleResponse, *ListScalesResponse, *ScaleCategoriesResponse, *ListHotScalesResponse]{
			DetailKey:       detailCacheKey,
			ListKey:         func(req any) string { return listCacheKey(req.(*ListScalesRequest)) },
			CategoriesKey:   cacheKeyCategories,
			HotKey:          func(req any) string { return hotCacheKey(req.(*ListHotScalesRequest)) },
			ListPrefix:      cacheKeyPrefixList,
			HotPrefix:       cacheKeyPrefixHot,
			CloneDetail:     cloneScaleResponse,
			CloneList:       cloneListScalesResponse,
			CloneCategories: cloneScaleCategoriesResponse,
			CloneHot:        cloneListHotScalesResponse,
		}),
	}
}

func detailCacheKey(code string) string {
	return cacheKeyPrefixDetail + strings.ToLower(strings.TrimSpace(code))
}

func listCacheKey(req *ListScalesRequest) string {
	if req == nil {
		req = &ListScalesRequest{}
	}
	return fmt.Sprintf("%sp%d:ps%d:st%s:t:%s:c:%s:sg:%s:ag:%s:rp:%s:tg:%s",
		cacheKeyPrefixList,
		req.Page, req.PageSize, req.Status, req.Title, req.Category,
		joinKeyParts(req.Stages), joinKeyParts(req.ApplicableAges),
		joinKeyParts(req.Reporters), joinKeyParts(req.Tags),
	)
}

func hotCacheKey(req *ListHotScalesRequest) string {
	if req == nil {
		req = &ListHotScalesRequest{}
	}
	return fmt.Sprintf("%sl%d:w%d", cacheKeyPrefixHot, req.Limit, req.WindowDays)
}

func joinKeyParts(parts []string) string {
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func (c *LocalCatalogCache) GetDetail(code string) (*ScaleResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.GetDetail(code)
}

func (c *LocalCatalogCache) SetDetail(code string, value *ScaleResponse) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.SetDetail(code, value)
}

func (c *LocalCatalogCache) GetList(key string) (*ListScalesResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.GetList(key)
}

func (c *LocalCatalogCache) SetList(key string, value *ListScalesResponse) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.SetList(key, value)
}

func (c *LocalCatalogCache) GetListByRequest(req *ListScalesRequest) (*ListScalesResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	if req == nil {
		req = &ListScalesRequest{}
	}
	return c.inner.GetListByRequest(req)
}

func (c *LocalCatalogCache) SetListByRequest(req *ListScalesRequest, value *ListScalesResponse) {
	if c == nil || c.inner == nil {
		return
	}
	if req == nil {
		req = &ListScalesRequest{}
	}
	c.inner.SetListByRequest(req, value)
}

func (c *LocalCatalogCache) GetCategories() (*ScaleCategoriesResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.GetCategories()
}

func (c *LocalCatalogCache) SetCategories(value *ScaleCategoriesResponse) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.SetCategories(value)
}

func (c *LocalCatalogCache) GetHot(key string) (*ListHotScalesResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.GetHot(key)
}

func (c *LocalCatalogCache) SetHot(key string, value *ListHotScalesResponse) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.SetHot(key, value)
}

func (c *LocalCatalogCache) GetHotByRequest(req *ListHotScalesRequest) (*ListHotScalesResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	if req == nil {
		req = &ListHotScalesRequest{}
	}
	return c.inner.GetHotByRequest(req)
}

func (c *LocalCatalogCache) SetHotByRequest(req *ListHotScalesRequest, value *ListHotScalesResponse) {
	if c == nil || c.inner == nil {
		return
	}
	if req == nil {
		req = &ListHotScalesRequest{}
	}
	c.inner.SetHotByRequest(req, value)
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
