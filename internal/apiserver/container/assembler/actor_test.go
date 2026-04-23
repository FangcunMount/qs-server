package assembler

import (
	"testing"
	"time"

	testeeCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestNewActorModuleRequiresMySQLDB(t *testing.T) {
	t.Parallel()

	if _, err := NewActorModule(ActorModuleDeps{}); err == nil {
		t.Fatal("NewActorModule() error = nil, want missing MySQL error")
	}
}

func TestNewActorModuleUsesCachedTesteeRepoWhenRedisConfigured(t *testing.T) {
	t.Parallel()

	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = redisClient.Close() })

	module, err := NewActorModule(ActorModuleDeps{
		MySQLDB:      &gorm.DB{},
		RedisClient:  redisClient,
		CacheBuilder: rediskey.NewBuilderWithNamespace("test"),
		TesteePolicy: cachepolicy.CachePolicy{TTL: time.Minute},
	})
	if err != nil {
		t.Fatalf("NewActorModule() error = %v", err)
	}
	if _, ok := module.TesteeRepo.(*testeeCache.CachedTesteeRepository); !ok {
		t.Fatalf("TesteeRepo type = %T, want *cache.CachedTesteeRepository", module.TesteeRepo)
	}
}
