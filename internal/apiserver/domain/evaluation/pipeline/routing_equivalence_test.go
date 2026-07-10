package pipeline_test

import (
	"testing"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestExecutionPathRoutingEquivalenceForModelKinds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind evaldomain.ModelKind
		want modelcatalog.ExecutionPath
	}{
		{evaldomain.ModelKindScale, modelcatalog.ExecutionPathScaleDescriptor},
		{evaldomain.ModelKindTypology, modelcatalog.ExecutionPathTypologyDescriptor},
		{evaldomain.ModelKindBehavioralRating, modelcatalog.ExecutionPathBehavioralRatingDescriptor},
		{evaldomain.ModelKindCognitive, modelcatalog.ExecutionPathCognitiveDescriptor},
	}
	for _, tc := range cases {
		desc := evaldomain.ModelDescriptor{Kind: tc.kind}
		fromDescriptor, err := evaldomain.ExecutionPathForDescriptor(desc)
		if err != nil {
			t.Fatalf("kind=%s ExecutionPathForDescriptor: %v", tc.kind, err)
		}
		fromPipeline, err := evalpipeline.ExecutionPathForModelKind(evalpipeline.ModelKind(tc.kind))
		if err != nil {
			t.Fatalf("kind=%s ExecutionPathForModelKind: %v", tc.kind, err)
		}
		if fromDescriptor != tc.want {
			t.Fatalf("kind=%s descriptor path=%s want=%s", tc.kind, fromDescriptor, tc.want)
		}
		if fromPipeline != tc.want {
			t.Fatalf("kind=%s pipeline path=%s want=%s", tc.kind, fromPipeline, tc.want)
		}
		if fromDescriptor != fromPipeline {
			t.Fatalf("kind=%s paths diverged: descriptor=%s pipeline=%s", tc.kind, fromDescriptor, fromPipeline)
		}
	}
}

func TestRuntimeDescriptorKeyMatchesIdentityDerivation(t *testing.T) {
	t.Parallel()

	cases := []evalpipeline.ModelRoute{
		{
			Kind:          modelcatalog.KindScale,
			Algorithm:     modelcatalog.AlgorithmScaleDefault,
			DecisionKind:  modelcatalog.DecisionKindScoreRange,
			PayloadFormat: modelcatalog.PayloadFormatAssessmentScaleV1,
		},
		{
			Kind:          modelcatalog.KindTypology,
			SubKind:       modelcatalog.SubKindTypology,
			Algorithm:     modelcatalog.AlgorithmMBTI,
			DecisionKind:  modelcatalog.DecisionKindPoleComposition,
			PayloadFormat: modelcatalog.PayloadFormatPersonalityTypologyV1,
		},
		{
			DecisionKind:  modelcatalog.DecisionKindNormLookup,
			PayloadFormat: modelcatalog.PayloadFormatBehavioralRatingBrief2V1,
		},
	}
	for i, route := range cases {
		key, err := evalpipeline.RuntimeDescriptorKeyFromRoute(route)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		family, ok := evalpipeline.AlgorithmFamilyFromRoute(route)
		if !ok {
			t.Fatalf("case %d: AlgorithmFamilyFromRoute failed", i)
		}
		if key.AlgorithmFamily != family {
			t.Fatalf("case %d: key family=%s route family=%s", i, key.AlgorithmFamily, family)
		}
		if route.DecisionKind != "" {
			wantFamily, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind)
			if !ok {
				t.Fatalf("case %d: decision kind %s", i, route.DecisionKind)
			}
			if key.AlgorithmFamily != wantFamily {
				t.Fatalf("case %d: family=%s want=%s", i, key.AlgorithmFamily, wantFamily)
			}
			if key.DecisionKind != route.DecisionKind {
				t.Fatalf("case %d: decision=%s want=%s", i, key.DecisionKind, route.DecisionKind)
			}
		}
	}
}

func TestRouteIdentityFamilyMatchesRuntimeFamily(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		route evalpipeline.ModelRoute
		want  modelcatalog.AlgorithmFamily
	}{
		{
			name: "scale",
			route: evalpipeline.ModelRoute{
				Kind:      modelcatalog.KindScale,
				Algorithm: modelcatalog.AlgorithmScaleDefault,
			},
			want: modelcatalog.AlgorithmFamilyFactorScoring,
		},
		{
			name: "typology",
			route: evalpipeline.ModelRoute{
				Kind:      modelcatalog.KindTypology,
				SubKind:   modelcatalog.SubKindTypology,
				Algorithm: modelcatalog.AlgorithmMBTI,
			},
			want: modelcatalog.AlgorithmFamilyFactorClassification,
		},
		{
			name: "behavioral_rating",
			route: evalpipeline.ModelRoute{
				Kind:      modelcatalog.KindBehavioralRating,
				Algorithm: modelcatalog.AlgorithmBrief2,
			},
			want: modelcatalog.AlgorithmFamilyFactorNorm,
		},
		{
			name: "cognitive_projection",
			route: evalpipeline.ModelRoute{
				Kind:      modelcatalog.KindCognitive,
				Algorithm: modelcatalog.AlgorithmSPM,
			},
			want: modelcatalog.AlgorithmFamilyTaskPerformance,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fromIdentity, ok := modelcatalog.AlgorithmFamilyFromIdentity(tc.route.Kind, tc.route.SubKind, tc.route.Algorithm)
			if !ok {
				t.Fatalf("AlgorithmFamilyFromIdentity(%s,%s,%s) ok = false", tc.route.Kind, tc.route.SubKind, tc.route.Algorithm)
			}
			fromRuntime, ok := evalpipeline.ExecutionFamilyFromRoute(tc.route)
			if !ok {
				t.Fatalf("ExecutionFamilyFromRoute(%s) ok = false", tc.name)
			}
			if fromIdentity != fromRuntime {
				t.Fatalf("family mismatch: identity=%s runtime=%s", fromIdentity, fromRuntime)
			}
			if fromRuntime != tc.want {
				t.Fatalf("family = %s, want %s", fromRuntime, tc.want)
			}
		})
	}
}

func TestRouteRoutingUsesCanonicalTypologyKind(t *testing.T) {
	t.Parallel()

	family, ok := evalpipeline.ExecutionFamilyFromRoute(evalpipeline.ModelRoute{Kind: modelcatalog.KindTypology})
	if !ok {
		t.Fatal("ExecutionFamilyFromRoute(typology) ok = false")
	}
	if family != modelcatalog.AlgorithmFamilyFactorClassification {
		t.Fatalf("family = %s, want %s", family, modelcatalog.AlgorithmFamilyFactorClassification)
	}
}

func TestRuntimeDescriptorKeyAlignsWithMechanismReportBuilderFamilyAndDecision(t *testing.T) {
	t.Parallel()

	cases := []struct {
		route evalpipeline.ModelRoute
	}{
		{
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindScoreRange},
		},
		{
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindPoleComposition},
		},
		{
			route: evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKindTraitProfile},
		},
	}
	for i, tc := range cases {
		key, err := evalpipeline.RuntimeDescriptorKeyFromRoute(tc.route)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(tc.route.DecisionKind)
		if !ok {
			t.Fatalf("case %d: decision %s", i, tc.route.DecisionKind)
		}
		if key.AlgorithmFamily != family {
			t.Fatalf("case %d: family=%s want=%s", i, key.AlgorithmFamily, family)
		}
		if key.DecisionKind != tc.route.DecisionKind {
			t.Fatalf("case %d: decision=%s want=%s", i, key.DecisionKind, tc.route.DecisionKind)
		}
	}
}
