package runtime

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// RegisteredEvaluatorPaths 返回执行路径 使用 evaluator 因子ies 按稳定顺序。
func RegisteredEvaluatorPaths() ([]modelcatalog.ExecutionPath, error) {
	return pathsInMaterializationOrder(keysOf(evaluatorFactories))
}

func keysOf[M ~map[modelcatalog.ExecutionPath]E, E any](factories M) map[modelcatalog.ExecutionPath]struct{} {
	keys := make(map[modelcatalog.ExecutionPath]struct{}, len(factories))
	for path := range factories {
		keys[path] = struct{}{}
	}
	return keys
}

func pathsInMaterializationOrder(factories map[modelcatalog.ExecutionPath]struct{}) ([]modelcatalog.ExecutionPath, error) {
	if len(factories) == 0 {
		return nil, fmt.Errorf("materialize factory map is empty")
	}
	paths := make([]modelcatalog.ExecutionPath, 0, len(factories))
	for _, path := range runtimeMaterializationOrder {
		if _, ok := factories[path]; ok {
			paths = append(paths, path)
		}
	}
	if len(paths) != len(factories) {
		return nil, fmt.Errorf("materialize factory map has unregistered execution paths")
	}
	return paths, nil
}
