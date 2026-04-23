package container

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
)

type fakeModule struct {
	info         assembler.ModuleInfo
	checkHealth  error
	cleanup      error
	checkCalls   int
	cleanupCalls int
}

func (*fakeModule) Initialize(...interface{}) error { return nil }

func (m *fakeModule) CheckHealth() error {
	m.checkCalls++
	return m.checkHealth
}

func (m *fakeModule) Cleanup() error {
	m.cleanupCalls++
	return m.cleanup
}

func (m *fakeModule) ModuleInfo() assembler.ModuleInfo { return m.info }

func TestContainerModulesAreInstanceScoped(t *testing.T) {
	t.Parallel()

	left := NewContainer(nil, nil, nil)
	right := NewContainer(nil, nil, nil)

	leftModule := &fakeModule{info: assembler.ModuleInfo{Name: "left"}}
	rightModule := &fakeModule{info: assembler.ModuleInfo{Name: "right"}}

	left.registerModule("left", leftModule)
	right.registerModule("right", rightModule)

	if err := left.checkModulesHealth(context.Background()); err != nil {
		t.Fatalf("left checkModulesHealth() error = %v", err)
	}
	if leftModule.checkCalls != 1 {
		t.Fatalf("left module check calls = %d, want 1", leftModule.checkCalls)
	}
	if rightModule.checkCalls != 0 {
		t.Fatalf("right module check calls = %d, want 0", rightModule.checkCalls)
	}

	if got := left.GetLoadedModules(); !reflect.DeepEqual(got, []string{"left"}) {
		t.Fatalf("left GetLoadedModules() = %v, want [left]", got)
	}
	if got := right.GetLoadedModules(); !reflect.DeepEqual(got, []string{"right"}) {
		t.Fatalf("right GetLoadedModules() = %v, want [right]", got)
	}
}

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
