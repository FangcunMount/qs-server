package assembler

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestNewActorModuleRequiresMySQLDB(t *testing.T) {
	t.Parallel()

	if _, err := NewActorModule(ActorModuleDeps{}); err == nil {
		t.Fatal("NewActorModule() error = nil, want missing MySQL error")
	}
}

func TestNewActorModuleAcceptsRedisConfiguredTesteeCache(t *testing.T) {
	t.Parallel()

	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = redisClient.Close() })

	module, err := NewActorModule(ActorModuleDeps{
		MySQLDB:      &gorm.DB{},
		RedisClient:  redisClient,
		CacheBuilder: keyspace.NewBuilderWithNamespace("test"),
		TesteePolicy: cachepolicy.CachePolicy{TTL: time.Minute},
	})
	if err != nil {
		t.Fatalf("NewActorModule() error = %v", err)
	}
	if module.TesteeRegistrationService == nil || module.TesteeQueryService == nil {
		t.Fatalf("actor module services were not initialized")
	}
}
