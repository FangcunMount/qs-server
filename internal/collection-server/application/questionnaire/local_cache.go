package questionnaire

import (
	"time"

	localcache "github.com/FangcunMount/qs-server/internal/pkg/cache/local"
)

// LocalCacheOptions 进程内 L1 缓存配置。
type LocalCacheOptions struct {
	TTL            time.Duration
	MaxEntries     int
	TTLJitterRatio float64
	OnHit          func()
	OnMiss         func()
}

// LocalCache 已发布问卷 REST DTO 的进程内 TTL 缓存。
type LocalCache struct {
	inner *localcache.DetailCache[*QuestionnaireResponse]
}

// NewLocalCache 创建进程内问卷详情缓存。
func NewLocalCache(opts LocalCacheOptions) *LocalCache {
	return &LocalCache{
		inner: localcache.NewDetailCache(localcache.Options{
			TTL:            opts.TTL,
			MaxEntries:     opts.MaxEntries,
			TTLJitterRatio: opts.TTLJitterRatio,
			OnHit:          opts.OnHit,
			OnMiss:         opts.OnMiss,
		}, localcache.DetailHooks[*QuestionnaireResponse]{
			KeyFn:  cacheKey,
			Clone:  cloneResponse,
			Prefix: "published:",
		}),
	}
}

func (c *LocalCache) Get(code, version string) (*QuestionnaireResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.Get(code, version)
}

func (c *LocalCache) Set(code, version string, value *QuestionnaireResponse) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.Set(code, version, value)
}

func (c *LocalCache) Delete(code, version string) {
	if c == nil || c.inner == nil {
		return
	}
	c.inner.Delete(code, version)
}

func (c *LocalCache) Stats() (hits, misses uint64) {
	if c == nil || c.inner == nil {
		return 0, 0
	}
	return c.inner.Stats()
}
