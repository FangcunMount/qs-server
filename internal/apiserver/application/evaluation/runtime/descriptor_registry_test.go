package runtime_test

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDefaultRuntimeDescriptorRegistryCoversMaterializePaths(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	if registry.Len() != 4 {
		t.Fatalf("registry len = %d, want 4", registry.Len())
	}
	cases := []struct {
		name string
		snap modelcatalog.PublishedModelSnapshot
		path modelcatalog.ExecutionPath
	}{
		{
			name: "scale",
			snap: modelcatalog.PublishedModelSnapshot{Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindScoreRange}},
			path: modelcatalog.ExecutionPathScaleDescriptor,
		},
		{
			name: "typology",
			snap: modelcatalog.PublishedModelSnapshot{Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition}},
			path: modelcatalog.ExecutionPathTypologyDescriptor,
		},
		{
			name: "norm",
			snap: modelcatalog.PublishedModelSnapshot{Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindNormLookup}},
			path: modelcatalog.ExecutionPathBehavioralRatingDescriptor,
		},
		{
			name: "task",
			snap: modelcatalog.PublishedModelSnapshot{Decision: modelcatalog.DecisionSpec{Kind: modelcatalog.DecisionKindAbilityLevel}},
			path: modelcatalog.ExecutionPathCognitiveDescriptor,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc, err := registry.Resolve(tc.snap)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if desc.ExecutionPath != tc.path {
				t.Fatalf("descriptor path = %s, want %s", desc.ExecutionPath, tc.path)
			}
		})
	}
}
