package runtime_test

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDefaultRuntimeDescriptorRegistryCoversMaterializePaths(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	if registry.Len() != 7 {
		t.Fatalf("registry len = %d, want 7", registry.Len())
	}
	cases := []struct {
		name  string
		route evalpipeline.ModelRoute
		path  modelcatalog.ExecutionPath
	}{
		{
			name:  "scale",
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindScoreRange},
			path:  modelcatalog.ExecutionPathScaleDescriptor,
		},
		{
			name:  "typology",
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindPoleComposition},
			path:  modelcatalog.ExecutionPathTypologyDescriptor,
		},
		{
			name:  "norm",
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindNormLookup},
			path:  modelcatalog.ExecutionPathBehavioralRatingDescriptor,
		},
		{
			name:  "task",
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindAbilityLevel},
			path:  modelcatalog.ExecutionPathCognitiveDescriptor,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc, err := registry.Resolve(tc.route)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if desc.ExecutionPath != tc.path {
				t.Fatalf("descriptor path = %s, want %s", desc.ExecutionPath, tc.path)
			}
		})
	}
}
