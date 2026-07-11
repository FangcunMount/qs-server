package runtime

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	portruleengine "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type evaluatorFactory func(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (execute.Evaluator, error)

var evaluatorFactories map[modelcatalog.ExecutionPath]evaluatorFactory

func init() {
	var err error
	evaluatorFactories, err = buildFactoryMaps(defaultPathMaterializations())
	if err != nil {
		panic("default materialization specs: " + err.Error())
	}
}

// WiringDeps 分组共享 运行时 物化依赖。
type WiringDeps struct {
	ScaleScorer      portruleengine.ScaleFactorScorer
	TypologyRegistry factorclassification.ModuleRegistry
}

type wiringSession struct {
	typologyExecutor **factorclassification.Executor
}

// MaterializeFamilyEvaluators 构建一个evaluator per 算法家族 从 物化 specs。
func MaterializeFamilyEvaluators(deps WiringDeps) (map[modelcatalog.AlgorithmFamily]execute.Evaluator, error) {
	var sharedConfigured *factorclassification.Executor
	session := wiringSession{typologyExecutor: &sharedConfigured}
	out := make(map[modelcatalog.AlgorithmFamily]execute.Evaluator, len(defaultPathMaterializations()))
	for _, spec := range defaultPathMaterializations() {
		desc := defaultDescriptorForExecutionPath(spec.path)
		evaluator, err := spec.evaluator(desc, deps, session)
		if err != nil {
			return nil, err
		}
		out[spec.family] = evaluator
	}
	return out, nil
}

func defaultDescriptorForExecutionPath(path modelcatalog.ExecutionPath) evaldomain.ModelDescriptor {
	switch path {
	case modelcatalog.ExecutionPathTypologyDescriptor:
		return factorclassification.ConfiguredTypologyDescriptor()
	default:
		return evaldomain.ModelDescriptor{Kind: modelKindForExecutionPath(path)}
	}
}

func modelKindForExecutionPath(path modelcatalog.ExecutionPath) evaldomain.ModelKind {
	switch path {
	case modelcatalog.ExecutionPathScaleDescriptor:
		return evaldomain.ModelKindScale
	case modelcatalog.ExecutionPathTypologyDescriptor:
		return evaldomain.ModelKindTypology
	case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
		return evaldomain.ModelKindBehavioralRating
	case modelcatalog.ExecutionPathCognitiveDescriptor:
		return evaldomain.ModelKindCognitive
	default:
		return ""
	}
}

// MaterializeEvaluators 构建evaluators 从 描述符。
func MaterializeEvaluators(descs []evaldomain.ModelDescriptor, deps WiringDeps) ([]execute.Evaluator, error) {
	var sharedConfigured *factorclassification.Executor
	session := wiringSession{typologyExecutor: &sharedConfigured}
	evaluators := make([]execute.Evaluator, 0, len(descs))
	for _, desc := range descs {
		evaluator, err := materializeEvaluator(desc, deps, session)
		if err != nil {
			return nil, err
		}
		evaluators = append(evaluators, evaluator)
	}
	return evaluators, nil
}

func materializeEvaluator(desc evaldomain.ModelDescriptor, deps WiringDeps, session wiringSession) (execute.Evaluator, error) {
	path, err := executionPathForDescriptor(desc)
	if err != nil {
		return nil, err
	}
	factory, ok := evaluatorFactories[path]
	if !ok {
		return nil, fmt.Errorf("unsupported evaluation execution path: %s", path)
	}
	return factory(desc, deps, session)
}

func requireTypologyRegistry(deps WiringDeps) (factorclassification.ModuleRegistry, error) {
	if deps.TypologyRegistry.Len() == 0 {
		return factorclassification.ModuleRegistry{}, fmt.Errorf("typology registry is required")
	}
	return deps.TypologyRegistry, nil
}

func executionPathForDescriptor(desc evaldomain.ModelDescriptor) (modelcatalog.ExecutionPath, error) {
	return evalpipeline.ExecutionPathForModelKind(evalpipeline.ModelKind(desc.Kind))
}
