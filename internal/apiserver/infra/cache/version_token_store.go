package cache

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	redis "github.com/redis/go-redis/v9"
)

type VersionTokenStore = cachequery.VersionTokenStore
type RedisVersionTokenStore = cachequery.RedisVersionTokenStore

func NewRedisVersionTokenStore(client redis.UniversalClient) VersionTokenStore {
	return cachequery.NewRedisVersionTokenStore(client)
}

func NewRedisVersionTokenStoreWithKind(client redis.UniversalClient, kind string) VersionTokenStore {
	return cachequery.NewRedisVersionTokenStoreWithKind(client, kind)
}

func NewRedisVersionTokenStoreWithKindAndObserver(client redis.UniversalClient, kind string, observer *Observer) VersionTokenStore {
	return cachequery.NewRedisVersionTokenStoreWithKindAndObserver(client, kind, observer)
}

func NewStaticVersionTokenStore(version uint64) VersionTokenStore {
	return cachequery.NewStaticVersionTokenStore(version)
}
