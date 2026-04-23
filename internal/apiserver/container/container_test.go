package container

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
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

func TestContainerBuildActorModuleInitializeParamsUsesObjectCacheBuilderAndPolicy(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	builder := rediskey.NewBuilderWithNamespace("actor")
	policy := cachepolicy.CachePolicy{TTL: 5}
	c.objectRedisHandle = &redisplane.Handle{Builder: builder}
	c.policyCatalog = cachepolicy.NewPolicyCatalog(nil, map[cachepolicy.CachePolicyKey]cachepolicy.CachePolicy{
		cachepolicy.PolicyTestee: policy,
	})

	params := c.buildActorModuleInitializeParams()
	if len(params) != 8 {
		t.Fatalf("len(params) = %d, want 8", len(params))
	}
	if params[4] != builder {
		t.Fatalf("cache builder = %#v, want %#v", params[4], builder)
	}
	gotPolicy, ok := params[5].(cachepolicy.CachePolicy)
	if !ok {
		t.Fatalf("policy arg type = %T, want cachepolicy.CachePolicy", params[5])
	}
	if gotPolicy != policy {
		t.Fatalf("policy = %#v, want %#v", gotPolicy, policy)
	}
	if !isNilInterfaceValue(params[1]) || !isNilInterfaceValue(params[2]) || !isNilInterfaceValue(params[6]) || !isNilInterfaceValue(params[7]) {
		t.Fatalf("unexpected IAM deps in params: %#v", params)
	}
}

func isNilInterfaceValue(value interface{}) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
