package personalitymodel

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/localttlcache"
)

const (
	cacheKeyPrefixDetail = "personality:detail:"
	cacheKeyCategories   = "personality:categories"
	cacheKeyPrefixList   = "personality:list:"
)

// LocalCatalogCache 人格模型目录进程内 TTL 缓存。
type LocalCatalogCache struct {
	detail     *localttlcache.Cache[*PersonalityModelResponse]
	list       *localttlcache.Cache[*ListPersonalityModelsResponse]
	categories *localttlcache.Cache[*PersonalityModelCategoriesResponse]
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
	if opts.TTL <= 0 {
		opts.TTL = defaultCatalogCacheTTLSeconds * time.Second
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 256
	}
	base := localttlcache.Options{
		TTL:            opts.TTL,
		MaxEntries:     opts.MaxEntries,
		TTLJitterRatio: opts.TTLJitterRatio,
		OnHit:          opts.OnHit,
		OnMiss:         opts.OnMiss,
	}
	return &LocalCatalogCache{
		detail:     localttlcache.New(base, clonePersonalityModelResponse),
		list:       localttlcache.New(base, cloneListPersonalityModelsResponse),
		categories: localttlcache.New(base, clonePersonalityModelCategoriesResponse),
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
	if c == nil || c.detail == nil {
		return nil, false
	}
	return c.detail.Get(detailCacheKey(code))
}

func (c *LocalCatalogCache) SetDetail(code string, value *PersonalityModelResponse) {
	if c == nil || c.detail == nil || value == nil {
		return
	}
	c.detail.Set(detailCacheKey(code), value)
}

func (c *LocalCatalogCache) GetListByRequest(req *ListPersonalityModelsRequest) (*ListPersonalityModelsResponse, bool) {
	if c == nil || c.list == nil {
		return nil, false
	}
	return c.list.Get(listCacheKey(req))
}

func (c *LocalCatalogCache) SetListByRequest(req *ListPersonalityModelsRequest, value *ListPersonalityModelsResponse) {
	if c == nil || c.list == nil || value == nil {
		return
	}
	c.list.Set(listCacheKey(req), value)
}

func (c *LocalCatalogCache) GetCategories() (*PersonalityModelCategoriesResponse, bool) {
	if c == nil || c.categories == nil {
		return nil, false
	}
	return c.categories.Get(cacheKeyCategories)
}

func (c *LocalCatalogCache) SetCategories(value *PersonalityModelCategoriesResponse) {
	if c == nil || c.categories == nil || value == nil {
		return
	}
	c.categories.Set(cacheKeyCategories, value)
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
}

func (c *LocalCatalogCache) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}
	for _, part := range []*localttlcache.Cache[*PersonalityModelResponse]{c.detail} {
		h, m := part.Stats()
		hits += h
		misses += m
	}
	for _, part := range []*localttlcache.Cache[*ListPersonalityModelsResponse]{c.list} {
		h, m := part.Stats()
		hits += h
		misses += m
	}
	for _, part := range []*localttlcache.Cache[*PersonalityModelCategoriesResponse]{c.categories} {
		h, m := part.Stats()
		hits += h
		misses += m
	}
	return hits, misses
}
