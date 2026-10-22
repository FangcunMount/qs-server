package container

import (
	"context"
	"errors"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"sync/atomic"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func TestContainerCleanupUsesRegisteredModules(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	first := &fakeModule{info: ModuleInfo{Name: "survey"}}
	second := &fakeModule{info: ModuleInfo{Name: "plan"}}
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
		info:        ModuleInfo{Name: "broken"},
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
	c.registerModule("survey", &fakeModule{info: ModuleInfo{Name: "survey", Version: "1.0.0"}})
	c.registerModule("plan", &fakeModule{info: ModuleInfo{Name: "plan", Version: "1.0.0"}})
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

// This module deliberately ignores caller cancellation while real work owns
// dependencies. The container must bound waits without creating more cleaners.
type drainingModule struct {
	fakeModule
	entered, release chan struct{}
	calls            atomic.Int32
}

func (m *drainingModule) Cleanup() error {
	m.calls.Add(1)
	close(m.entered)
	<-m.release
	return nil
}
func TestCleanupTimeoutRetainsOneRealCoordinatorUntilModuleDrains(t *testing.T) {
	c := NewContainerWithOptions(nil, nil, nil, ContainerOptions{Silent: true})
	module := &drainingModule{entered: make(chan struct{}), release: make(chan struct{})}
	c.registerModule("draining", module)
	c.initialized = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 8; i++ {
		err := c.CleanupContext(ctx)
		var pending *eventsubsystem.ShutdownIncomplete
		if !errors.As(err, &pending) || pending.PendingClosures != 1 || !errors.Is(err, context.Canceled) {
			t.Fatalf("cleanup returned false completion: %v", err)
		}
	}
	select {
	case <-module.entered:
	case <-time.After(time.Second):
		t.Fatal("real coordinator not running")
	}
	if module.calls.Load() != 1 {
		t.Fatal("repeated cleanup created another drain")
	}
	close(module.release)
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if err := c.CleanupContext(wait); err != nil {
		t.Fatal(err)
	}
	if c.initialized || module.calls.Load() != 1 {
		t.Fatal("completion or ownership was lost")
	}
	// Completion wins over an already cancelled subsequent waiter.
	if err := c.CleanupContext(ctx); err != nil {
		t.Fatal(err)
	}
}

type contextualModule struct {
	fakeModule
	contextual bool
	event      *eventsubsystem.Subsystem
}

func (m *contextualModule) CleanupContext(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Closing owner runtime before its business module is an ordering violation.
	if m.event.Wait(ctx) == nil {
		return errors.New("owner closed before consumer")
	}
	m.contextual = true
	return nil
}
func TestTransactionCleanupDrainsModulesBeforeClosingOwnerSubsystem(t *testing.T) {
	c := NewContainerWithOptions(nil, nil, nil, ContainerOptions{Silent: true})
	c.transactions = &EventTransactionBindings{}
	c.eventSubsystem = &eventsubsystem.Subsystem{}
	module := &contextualModule{event: c.eventSubsystem}
	c.registerModule("ai", module)
	if err := c.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if !module.contextual || module.cleanupCalls != 0 {
		t.Fatal("transaction consumer ignored shared drain context")
	}
	if err := c.eventSubsystem.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestLegacyCleanupPreservesModuleOwnBudget(t *testing.T) {
	c := NewContainerWithOptions(nil, nil, nil, ContainerOptions{Silent: true})
	module := &contextualModule{}
	c.registerModule("legacy", module)
	if err := c.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if module.contextual || module.cleanupCalls != 1 {
		t.Fatal("legacy module cleanup policy changed")
	}
}

func TestClosedContainerCannotInitializeNewResources(t *testing.T) {
	c := NewContainerWithOptions(nil, nil, nil, ContainerOptions{Silent: true})
	if err := c.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := c.Initialize(); err == nil {
		t.Fatal("closed coordinator allowed another resource generation")
	}
}
