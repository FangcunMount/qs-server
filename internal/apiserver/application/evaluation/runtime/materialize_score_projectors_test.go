package runtime_test

import (
	"context"
	"testing"

	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_classification"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	modelcatalogwire "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

func TestMaterializeScoreProjectorsRegistersScaleLikeRuntimes(t *testing.T) {
	t.Parallel()

	registry, err := modelcatalogwire.DefaultTypologyRegistry()
	if err != nil {
		t.Fatalf("DefaultTypologyRegistry: %v", err)
	}
	descs := modelcatalogwire.DefaultEvaluationDescriptors()
	projectors, err := evalruntime.MaterializeScoreProjectors(descs, evalruntime.WiringDeps{
		ScoreRepo:        noopScoreRepo{},
		TypologyRegistry: registry,
	})
	if err != nil {
		t.Fatalf("MaterializeScoreProjectors: %v", err)
	}
	keys := make(map[evaldomain.EvaluatorKey]bool, len(projectors))
	for _, projector := range projectors {
		keys[projector.Key()] = true
	}
	if !keys[evaldomain.EvaluatorKeyScaleDefault] {
		t.Fatal("scale score projector not materialized")
	}
	if !keys[evaldomain.EvaluatorKeyBehavioralRatingDefault] {
		t.Fatal("behavioral_rating score projector not materialized")
	}
	if !keys[evaldomain.EvaluatorKeyCognitiveDefault] {
		t.Fatal("cognitive score projector not materialized")
	}
	if len(projectors) != 3 {
		t.Fatalf("projector count = %d, want 3", len(projectors))
	}
	_ = typologyeval.DefaultModules()
}

type noopScoreRepo struct{}

func (noopScoreRepo) SaveScoresWithContext(context.Context, *assessment.Assessment, *assessment.ScaleScoreProjection) error {
	return nil
}
func (noopScoreRepo) DeleteByAssessmentID(context.Context, assessment.ID) error { return nil }
