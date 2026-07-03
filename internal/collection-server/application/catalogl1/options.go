package catalogl1

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/localttlcache"
)

// Options 进程内 L1 缓存配置。
type Options struct {
	TTL            time.Duration
	MaxEntries     int
	TTLJitterRatio float64
	OnHit          func()
	OnMiss         func()
}

func (o Options) withDefaults(defaultTTL time.Duration, defaultEntries int) Options {
	if o.TTL <= 0 {
		o.TTL = defaultTTL
	}
	if o.MaxEntries <= 0 {
		o.MaxEntries = defaultEntries
	}
	return o
}

func (o Options) localTTL(clone any) localttlcache.Options {
	return localttlcache.Options{
		TTL:            o.TTL,
		MaxEntries:     o.MaxEntries,
		TTLJitterRatio: o.TTLJitterRatio,
		OnHit:          o.OnHit,
		OnMiss:         o.OnMiss,
	}
}
