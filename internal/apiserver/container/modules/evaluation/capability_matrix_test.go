package evaluation_test

import (
	"testing"

	modelcatalogmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	modelcatalogwire "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
)

func TestEvaluationModuleMaterializesOnlyDeclaredDescriptors(t *testing.T) {
	t.Parallel()

	descs := modelcatalogwire.DefaultEvaluationDescriptors()
	registry, err := modelcatalogwire.DefaultTypologyRegistry()
	if err != nil {
		t.Fatalf("DefaultTypologyRegistry() error = %v", err)
	}

	evaluators, err := modelcatalogmod.MaterializeEvaluators(descs, modelcatalogmod.WiringDeps{
		ScaleReportBuilder: report.NewDefaultInterpretReportBuilder(nil),
		ScaleScorer:        ruleengine.NewScaleFactorScorer(),
		TypologyRegistry:   registry,
	})
	if err != nil {
		t.Fatalf("MaterializeEvaluators: %v", err)
	}

	if len(evaluators) != len(descs) {
		t.Fatalf("evaluator count = %d, want %d", len(evaluators), len(descs))
	}

	keys := make(map[evaldomain.EvaluatorKey]bool, len(evaluators))
	for _, evaluator := range evaluators {
		keys[evaluator.Key()] = true
	}
	if !keys[evaldomain.EvaluatorKeyScaleDefault] {
		t.Fatal("scale evaluator not materialized")
	}
	if !keys[evaldomain.EvaluatorKeyPersonalityTypology] {
		t.Fatal("configured typology evaluator not materialized")
	}
	if !keys[evaldomain.EvaluatorKeyBehavioralRatingDefault] {
		t.Fatal("behavioral_rating evaluator not materialized")
	}
	for _, forbidden := range []domain.Kind{
		domain.KindBehaviorAbility,
		domain.KindCognitive,
		domain.KindCustom,
	} {
		for key := range keys {
			if key.Kind == forbidden {
				t.Fatalf("unexpected evaluator for %q: %#v", forbidden, key)
			}
		}
	}
}
