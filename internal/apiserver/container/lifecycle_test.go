package container

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func TestContainerCleanupUsesRegisteredModules(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	first := &fakeModule{info: assembler.ModuleInfo{Name: "survey"}}
	second := &fakeModule{info: assembler.ModuleInfo{Name: "plan"}}
	c.registerModule("survey", first)
	c.registerModule("plan", second)
	c.initialized = true

	if err := c.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if first.cleanupCalls != 1 || second.cleanupCalls != 1 {
		t.Fatalf("cleanup calls = (%d, %d), want (1, 1)", first.cleanupCalls, second.cleanupCalls)
	}
	if c.initialized {
		t.Fatal("container initialized = true, want false")
	}
}

func TestContainerCheckModulesHealthReturnsModuleError(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	want := errors.New("boom")
	c.registerModule("broken", &fakeModule{
		info:        assembler.ModuleInfo{Name: "broken"},
		checkHealth: want,
	})

	if err := c.checkModulesHealth(context.Background()); err == nil || !errors.Is(err, want) {
		t.Fatalf("checkModulesHealth() error = %v, want wrapped %v", err, want)
	}
}

func TestContainerGetContainerInfoReflectsModulesAndInfrastructure(t *testing.T) {
	t.Parallel()

	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = redisClient.Close() })

	c := NewContainer(&gorm.DB{}, &mongo.Database{}, redisClient)
	c.registerModule("survey", &fakeModule{info: assembler.ModuleInfo{Name: "survey", Version: "1.0.0"}})
	c.registerModule("plan", &fakeModule{info: assembler.ModuleInfo{Name: "plan", Version: "1.0.0"}})
	c.initialized = true

	info := c.GetContainerInfo()
	if got := info["initialized"]; got != true {
		t.Fatalf("initialized = %#v, want true", got)
	}
	infra, ok := info["infrastructure"].(map[string]bool)
	if !ok {
		t.Fatalf("infrastructure type = %T, want map[string]bool", info["infrastructure"])
	}
	if !infra["mysql"] || !infra["mongodb"] || !infra["redis"] {
		t.Fatalf("infrastructure = %#v, want all backends present", infra)
	}
	modules, ok := info["modules"].(map[string]interface{})
	if !ok {
		t.Fatalf("modules type = %T, want map[string]interface{}", info["modules"])
	}
	if _, exists := modules["survey"]; !exists {
		t.Fatalf("modules = %#v, want survey entry", modules)
	}
	if _, exists := modules["plan"]; !exists {
		t.Fatalf("modules = %#v, want plan entry", modules)
	}
}
