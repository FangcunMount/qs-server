package redisadapter

import (
	baseredisadapter "github.com/FangcunMount/component-base/pkg/ratelimit/redisadapter"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	redis "github.com/redis/go-redis/v9"
)

// NewBackend creates a Redis-backed token bucket backend.
func NewBackend(client redis.UniversalClient, builder *keyspace.Builder) ratelimit.Backend {
	var keyFunc baseredisadapter.KeyFunc
	if builder != nil {
		keyFunc = builder.BuildLockKey
	}
	return baseredisadapter.NewBackend(client, keyFunc)
}
