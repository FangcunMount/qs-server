package actor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestNewRequiresMySQLDB(t *testing.T) {
	t.Parallel()

	if _, err := actor.New(actor.Deps{}); err == nil {
		t.Fatal("New() error = nil, want missing MySQL error")
	}
}

func TestNewAcceptsRedisConfiguredTesteeCache(t *testing.T) {
	t.Parallel()

	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = redisClient.Close() })

	module, err := actor.New(actor.Deps{
		MySQLDB:       &gorm.DB{},
		RedisClient:   redisClient,
		CacheBuilder:  keyspace.NewBuilderWithNamespace("test"),
		CachePolicies: sharedcache.NewRegistry(sharedcache.EffectiveCapability{Capability: cachepolicy.CapabilityActorTestee}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if module.TesteeRegistrationService == nil || module.TesteeQueryService == nil {
		t.Fatalf("actor module services were not initialized")
	}
}
