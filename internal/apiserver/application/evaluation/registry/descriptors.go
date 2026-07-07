package registry

import (
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// 默认EvaluationDescriptors 返回运行时描述符 用于 全部能力-backed 执行路径。
func DefaultEvaluationDescriptors() []evaldomain.ModelDescriptor {
	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		panic("default runtime descriptor registry: " + err.Error())
	}
	paths, err := evalruntime.ExecutionPathsFromRegistry(registry)
	if err != nil {
		panic("execution paths from registry: " + err.Error())
	}
	paths = evalruntime.FilterExecutablePaths(paths)

	descs := make([]evaldomain.ModelDescriptor, 0, len(paths)+4)
	for _, path := range paths {
		descs = append(descs, descriptorsForExecutionPath(path)...)
	}
	return descs
}

func descriptorsForExecutionPath(path modelcatalog.ExecutionPath) []evaldomain.ModelDescriptor {
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return []evaldomain.ModelDescriptor{evaldomain.ScaleModelDescriptor()}
	case modelcatalog.ExecutionPathTypologyDescriptor:
		return DefaultTypologyDescriptors()
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return []evaldomain.ModelDescriptor{evaldomain.BehavioralRatingModelDescriptor()}
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return []evaldomain.ModelDescriptor{evaldomain.CognitiveModelDescriptor()}
	default:
		return nil
	}
}
