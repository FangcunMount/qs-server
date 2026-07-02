package questionnaire

import (
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/localttlcache"
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
	inner *localttlcache.Cache[*QuestionnaireResponse]
}

// NewLocalCache 创建进程内问卷详情缓存。
func NewLocalCache(opts LocalCacheOptions) *LocalCache {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = defaultLocalCacheTTLSeconds * time.Second
	}
	return &LocalCache{
		inner: localttlcache.New(localttlcache.Options{
			TTL:            ttl,
			MaxEntries:     opts.MaxEntries,
			TTLJitterRatio: opts.TTLJitterRatio,
			OnHit:          opts.OnHit,
			OnMiss:         opts.OnMiss,
		}, cloneResponse),
	}
}

func (c *LocalCache) Get(code, version string) (*QuestionnaireResponse, bool) {
	if c == nil || c.inner == nil {
		return nil, false
	}
	return c.inner.Get(cacheKey(code, version))
}

func (c *LocalCache) Set(code, version string, value *QuestionnaireResponse) {
	if c == nil || c.inner == nil || value == nil {
		return
	}
	c.inner.Set(cacheKey(code, version), value)
}

func (c *LocalCache) Delete(code, version string) {
	if c == nil || c.inner == nil {
		return
	}
	code = strings.ToLower(strings.TrimSpace(code))
	version = strings.TrimSpace(version)
	if version == "" {
		c.inner.DeletePrefix("published:" + code)
		return
	}
	c.inner.Delete(cacheKey(code, version))
}

func (c *LocalCache) Stats() (hits, misses uint64) {
	if c == nil || c.inner == nil {
		return 0, 0
	}
	return c.inner.Stats()
}
