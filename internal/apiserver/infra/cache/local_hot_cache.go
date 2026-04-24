package cache

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
)

type LocalHotCache[T any] = cachequery.LocalHotCache[T]

func NewLocalHotCache[T any](ttl time.Duration, maxEntries int) *LocalHotCache[T] {
	return cachequery.NewLocalHotCache[T](ttl, maxEntries)
}
