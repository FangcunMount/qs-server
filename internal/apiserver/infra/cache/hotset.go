package cache

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachehotset"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

type HotsetRecorder = cachehotset.Recorder
type HotsetInspector = cachehotset.Inspector
type HotsetOptions = cachehotset.Options

func NewRedisHotsetStore(client redis.UniversalClient, builder *rediskey.Builder, opts HotsetOptions) HotsetRecorder {
	return cachehotset.NewRedisStore(client, builder, opts)
}

func NewRedisHotsetStoreWithObserver(client redis.UniversalClient, builder *rediskey.Builder, opts HotsetOptions, observer *Observer) HotsetRecorder {
	return cachehotset.NewRedisStoreWithObserver(client, builder, opts, observer)
}
