package runtime

import (
	"fmt"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

var runtimeMaterializationOrder = materializationOrder()

// ExecutionPathsFromRegistry 返回已注册 执行路径 in 稳定 物化 order。
func ExecutionPathsFromRegistry(registry *evalpipeline.RuntimeDescriptorRegistry) ([]modelcatalog.ExecutionPath, error) {
	if registry == nil {
		return nil, fmt.Errorf("runtime descriptor registry is nil")
	}
	paths := make([]modelcatalog.ExecutionPath, 0, registry.Len())
	for _, path := range runtimeMaterializationOrder {
		family, ok := algorithmFamilyForExecutionPath(path)
		if !ok {
			continue
		}
		if registry.HasAlgorithmFamily(family) {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("runtime descriptor registry has no supported execution paths")
	}
	return paths, nil
}

// FilterExecutablePaths 保留仅 paths 基于 运行时-可执行 model 能力。
func FilterExecutablePaths(paths []modelcatalog.ExecutionPath) []modelcatalog.ExecutionPath {
	executable := make(map[modelcatalog.ExecutionPath]bool)
	for _, kind := range modelcatalog.RuntimeExecutableKinds() {
		cap, ok := modelcatalog.FamilyCapabilityByKind(kind)
		if !ok || !cap.RuntimeExecutable {
			continue
		}
		executable[cap.ExecutionPath] = true
	}
	filtered := make([]modelcatalog.ExecutionPath, 0, len(paths))
	for _, path := range paths {
		if executable[path] {
			filtered = append(filtered, path)
		}
	}
	return filtered
}
