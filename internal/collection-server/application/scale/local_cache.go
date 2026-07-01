package scale

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/localttlcache"
)

const (
	cacheKeyPrefixDetail = "scale:detail:"
	cacheKeyCategories   = "scale:categories"
	cacheKeyPrefixList   = "scale:list:"
	cacheKeyPrefixHot    = "scale:hot:"
)

// LocalCatalogCache 量表目录进程内 TTL 缓存。
type LocalCatalogCache struct {
	detail     *localttlcache.Cache[*ScaleResponse]
	list       *localttlcache.Cache[*ListScalesResponse]
	categories *localttlcache.Cache[*ScaleCategoriesResponse]
	hot        *localttlcache.Cache[*ListHotScalesResponse]
}

// LocalCatalogCacheOptions 量表目录 L1 配置。
type LocalCatalogCacheOptions struct {
	TTL        time.Duration
	MaxEntries int
}

// NewLocalCatalogCache 创建量表目录 L1 缓存。
func NewLocalCatalogCache(opts LocalCatalogCacheOptions) *LocalCatalogCache {
	if opts.TTL <= 0 {
		opts.TTL = defaultCatalogCacheTTLSeconds * time.Second
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 256
	}
	base := localttlcache.Options{TTL: opts.TTL, MaxEntries: opts.MaxEntries}
	return &LocalCatalogCache{
		detail:     localttlcache.New(base, cloneScaleResponse),
		list:       localttlcache.New(base, cloneListScalesResponse),
		categories: localttlcache.New(base, cloneScaleCategoriesResponse),
		hot:        localttlcache.New(base, cloneListHotScalesResponse),
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
	if c == nil || c.detail == nil {
		return nil, false
	}
	return c.detail.Get(detailCacheKey(code))
}

func (c *LocalCatalogCache) SetDetail(code string, value *ScaleResponse) {
	if c == nil || c.detail == nil || value == nil {
		return
	}
	c.detail.Set(detailCacheKey(code), value)
}

func (c *LocalCatalogCache) GetList(key string) (*ListScalesResponse, bool) {
	if c == nil || c.list == nil {
		return nil, false
	}
	return c.list.Get(key)
}

func (c *LocalCatalogCache) SetList(key string, value *ListScalesResponse) {
	if c == nil || c.list == nil || value == nil {
		return
	}
	c.list.Set(key, value)
}

func (c *LocalCatalogCache) GetListByRequest(req *ListScalesRequest) (*ListScalesResponse, bool) {
	return c.GetList(listCacheKey(req))
}

func (c *LocalCatalogCache) SetListByRequest(req *ListScalesRequest, value *ListScalesResponse) {
	c.SetList(listCacheKey(req), value)
}

func (c *LocalCatalogCache) GetCategories() (*ScaleCategoriesResponse, bool) {
	if c == nil || c.categories == nil {
		return nil, false
	}
	return c.categories.Get(cacheKeyCategories)
}

func (c *LocalCatalogCache) SetCategories(value *ScaleCategoriesResponse) {
	if c == nil || c.categories == nil || value == nil {
		return
	}
	c.categories.Set(cacheKeyCategories, value)
}

func (c *LocalCatalogCache) GetHot(key string) (*ListHotScalesResponse, bool) {
	if c == nil || c.hot == nil {
		return nil, false
	}
	return c.hot.Get(key)
}

func (c *LocalCatalogCache) SetHot(key string, value *ListHotScalesResponse) {
	if c == nil || c.hot == nil || value == nil {
		return
	}
	c.hot.Set(key, value)
}

func (c *LocalCatalogCache) GetHotByRequest(req *ListHotScalesRequest) (*ListHotScalesResponse, bool) {
	return c.GetHot(hotCacheKey(req))
}

func (c *LocalCatalogCache) SetHotByRequest(req *ListHotScalesRequest, value *ListHotScalesResponse) {
	c.SetHot(hotCacheKey(req), value)
}

func (c *LocalCatalogCache) EvictOnSignal(code string) {
	if c == nil {
		return
	}
	code = strings.ToLower(strings.TrimSpace(code))
	if code != "" && c.detail != nil {
		c.detail.Delete(detailCacheKey(code))
	}
	if c.list != nil {
		c.list.DeletePrefix(cacheKeyPrefixList)
	}
	if c.categories != nil {
		c.categories.Delete(cacheKeyCategories)
	}
	if c.hot != nil {
		c.hot.DeletePrefix(cacheKeyPrefixHot)
	}
}

func (c *LocalCatalogCache) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}
	for _, part := range []*localttlcache.Cache[*ScaleResponse]{c.detail} {
		h, m := part.Stats()
		hits += h
		misses += m
	}
	for _, part := range []*localttlcache.Cache[*ListScalesResponse]{c.list} {
		h, m := part.Stats()
		hits += h
		misses += m
	}
	for _, part := range []*localttlcache.Cache[*ScaleCategoriesResponse]{c.categories} {
		h, m := part.Stats()
		hits += h
		misses += m
	}
	for _, part := range []*localttlcache.Cache[*ListHotScalesResponse]{c.hot} {
		h, m := part.Stats()
		hits += h
		misses += m
	}
	return hits, misses
}
