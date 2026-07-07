package runtime_test

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestExecutionPathsFromRegistryMatchesMaterializationOrder(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	paths, err := evalruntime.ExecutionPathsFromRegistry(registry)
	if err != nil {
		t.Fatalf("ExecutionPathsFromRegistry: %v", err)
	}
	want := []modelcatalog.ExecutionPath{
		modelcatalog.ExecutionPathScaleDescriptor,
		modelcatalog.ExecutionPathTypologyDescriptor,
		modelcatalog.ExecutionPathBehavioralRatingDescriptor,
		modelcatalog.ExecutionPathCognitiveDescriptor,
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	for i, path := range want {
		if paths[i] != path {
			t.Fatalf("paths[%d] = %s, want %s", i, paths[i], path)
		}
	}
}

func TestEvaluationDescriptorsFromRegistryProjectsLegacyDescriptors(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	descs, err := evalruntime.EvaluationDescriptorsFromRegistry(registry, func(path modelcatalog.ExecutionPath) []evaldomain.ModelDescriptor {
		switch path {
		case modelcatalog.ExecutionPathScaleDescriptor:
			return []evaldomain.ModelDescriptor{evaldomain.ScaleModelDescriptor()}
		case modelcatalog.ExecutionPathTypologyDescriptor:
			return []evaldomain.ModelDescriptor{{Key: evaldomain.EvaluatorKeyPersonalityTypology}}
		case modelcatalog.ExecutionPathBehavioralRatingDescriptor:
			return []evaldomain.ModelDescriptor{evaldomain.BehavioralRatingModelDescriptor()}
		case modelcatalog.ExecutionPathCognitiveDescriptor:
			return []evaldomain.ModelDescriptor{evaldomain.CognitiveModelDescriptor()}
		default:
			return nil
		}
	})
	if err != nil {
		t.Fatalf("EvaluationDescriptorsFromRegistry: %v", err)
	}
	if len(descs) != 4 {
		t.Fatalf("descriptor count = %d, want 4", len(descs))
	}
	if descs[0].Kind != evaldomain.ModelKindScale {
		t.Fatalf("first kind = %s, want scale", descs[0].Kind)
	}
	if descs[1].Key != evaldomain.EvaluatorKeyPersonalityTypology {
		t.Fatalf("typology key = %#v", descs[1].Key)
	}
}
