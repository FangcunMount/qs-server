package runtime

import (
	"fmt"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DescriptorProjector maps an execution path to legacy evaluator descriptors for wiring.
type DescriptorProjector func(path modelcatalog.ExecutionPath) []evaldomain.ModelDescriptor

var runtimeMaterializationOrder = []modelcatalog.ExecutionPath{
	modelcatalog.ExecutionPathScaleDescriptor,
	modelcatalog.ExecutionPathTypologyDescriptor,
	modelcatalog.ExecutionPathBehavioralRatingDescriptor,
	modelcatalog.ExecutionPathCognitiveDescriptor,
}

// ExecutionPathsFromRegistry returns registered execution paths in stable materialization order.
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

// EvaluationDescriptorsFromRegistry projects registered execution paths into evaluator descriptors.
func EvaluationDescriptorsFromRegistry(
	registry *evalpipeline.RuntimeDescriptorRegistry,
	project DescriptorProjector,
) ([]evaldomain.ModelDescriptor, error) {
	if project == nil {
		return nil, fmt.Errorf("descriptor projector is required")
	}
	paths, err := ExecutionPathsFromRegistry(registry)
	if err != nil {
		return nil, err
	}
	descs := make([]evaldomain.ModelDescriptor, 0, len(paths))
	for _, path := range paths {
		descs = append(descs, project(path)...)
	}
	return descs, nil
}

// FilterExecutablePaths keeps only paths backed by runtime-executable model capabilities.
func FilterExecutablePaths(paths []modelcatalog.ExecutionPath) []modelcatalog.ExecutionPath {
	executable := make(map[modelcatalog.ExecutionPath]bool)
	for _, cap := range modelcatalog.ModelFamilyCapabilitiesV2() {
		if cap.RuntimeExecutable {
			executable[cap.ExecutionPath] = true
		}
	}
	filtered := make([]modelcatalog.ExecutionPath, 0, len(paths))
	for _, path := range paths {
		if executable[path] {
			filtered = append(filtered, path)
		}
	}
	return filtered
}
