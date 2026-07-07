package runtime

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// RegisteredEvaluatorPaths returns execution paths with evaluator factories in stable order.
func RegisteredEvaluatorPaths() ([]modelcatalog.ExecutionPath, error) {
	return pathsInMaterializationOrder(keysOf(evaluatorFactories))
}

// RegisteredReportBuilderPaths returns execution paths with report builder factories in stable order.
func RegisteredReportBuilderPaths() ([]modelcatalog.ExecutionPath, error) {
	return pathsInMaterializationOrder(keysOf(reportBuilderFactories))
}

// RegisteredScoreProjectorPaths returns execution paths with score projector factories in stable order.
func RegisteredScoreProjectorPaths() ([]modelcatalog.ExecutionPath, error) {
	return pathsInMaterializationOrder(keysOf(scoreProjectorFactories))
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
