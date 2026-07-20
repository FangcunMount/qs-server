package outcome

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestModelRouteFromInputPreservesFrozenRuntimeIdentity(t *testing.T) {
	t.Parallel()

	route, ok := ModelRouteFromInput(&evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:            evaluationinput.EvaluationModelKindBehavioralRating,
			Algorithm:       string(modelcatalog.AlgorithmBehavioralRatingDefault),
			AlgorithmFamily: string(modelcatalog.AlgorithmFamilyFactorNorm),
			DecisionKind:    string(modelcatalog.DecisionKindNormLookup),
			PayloadFormat:   modelcatalog.PayloadFormatBehavioralRatingDefaultV1,
			ProductChannel:  string(modelcatalog.ProductChannel("screening")),
			Code:            "BR-001",
			Version:         "1.0.0",
			Title:           "筛查行为评分",
		},
	})
	if !ok {
		t.Fatal("ModelRouteFromInput returned false")
	}
	if route.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorNorm ||
		route.DecisionKind != modelcatalog.DecisionKindNormLookup ||
		route.PayloadFormat != modelcatalog.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("frozen runtime not preserved: %#v", route)
	}
}

func TestModelRouteFromInputPreservesRuntimeIdentity(t *testing.T) {
	t.Parallel()

	route, ok := ModelRouteFromInput(&evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:           evaluationinput.EvaluationModelKindBehavioralRating,
			Algorithm:      string(modelcatalog.AlgorithmBehavioralRatingDefault),
			ProductChannel: string(modelcatalog.ProductChannel("screening")),
			Code:           "BR-001",
			Version:        "1.0.0",
			Title:          "筛查行为评分",
		},
	})
	if !ok {
		t.Fatal("ModelRouteFromInput returned false")
	}
	if route.Kind != modelcatalog.KindBehavioralRating || route.Algorithm != modelcatalog.AlgorithmBehavioralRatingDefault {
		t.Fatalf("route identity = %s/%s", route.Kind, route.Algorithm)
	}
}

func TestModelRouteFromInputPreservesScaleIdentity(t *testing.T) {
	t.Parallel()

	route, ok := ModelRouteFromInput(&evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindScale,
			Algorithm: string(modelcatalog.AlgorithmScaleDefault),
			Code:      "PHQ9",
			Version:   "1.0.0",
			Title:     "PHQ-9",
		},
	})
	if !ok {
		t.Fatal("ModelRouteFromInput returned false")
	}
	if route.Kind != modelcatalog.KindScale || route.Algorithm != modelcatalog.AlgorithmScaleDefault {
		t.Fatalf("route identity = %s/%s", route.Kind, route.Algorithm)
	}
}
