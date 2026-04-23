package container

import (
	"context"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
)

func TestContainerModulesAreInstanceScoped(t *testing.T) {
	t.Parallel()

	left := NewContainer(nil, nil, nil)
	right := NewContainer(nil, nil, nil)

	leftModule := &fakeModule{info: moduleInfo("left")}
	rightModule := &fakeModule{info: moduleInfo("right")}

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

func moduleInfo(name string) assembler.ModuleInfo {
	return assembler.ModuleInfo{Name: name}
}
