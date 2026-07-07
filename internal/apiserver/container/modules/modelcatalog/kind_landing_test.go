package modelcatalog

import (
	"context"
	"testing"

	evaluationmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
)

// Kind landing contract: every RuntimeExecutable capability must materialize evaluator/builder/provider/score-projector.
func TestRuntimeExecutableKindsSatisfyLandingContract(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	registry, err := DefaultTypologyRegistry()
	if err != nil {
		t.Fatalf("DefaultTypologyRegistry: %v", err)
	}
	wiringDeps := evaluationmod.WiringDeps{
		ScaleReportBuilder: report.NewDefaultInterpretReportBuilder(nil),
		ScaleScorer:        ruleengine.NewScaleFactorScorer(),
		ScoreRepo:          kindLandingNoopScoreRepo{},
		TypologyRegistry:   registry,
	}

	evaluators, err := evaluationmod.MaterializeEvaluators(descs, wiringDeps)
	if err != nil {
		t.Fatalf("MaterializeEvaluators: %v", err)
	}
	builders, err := evaluationmod.MaterializeReportBuilders(descs, wiringDeps)
	if err != nil {
		t.Fatalf("MaterializeReportBuilders: %v", err)
	}
	projectors, err := evaluationmod.MaterializeScoreProjectors(descs, wiringDeps)
	if err != nil {
		t.Fatalf("MaterializeScoreProjectors: %v", err)
	}
	providers, err := evaluationinputInfra.MaterializeInputProviders(descs, evaluationinputInfra.InputProviderDeps{
		ScaleCatalog:            evalFakeScaleCatalog{},
		TypologyCatalog:         evalFakeTypologyCatalogPort{},
		BehavioralRatingCatalog: evalFakeBehavioralRatingCatalog{},
		CognitiveCatalog:        evalFakeCognitiveCatalog{},
		AnswerSheets:            evalFakeAnswerSheetReader{},
		Questionnaires:          evalFakeQuestionnaireReader{},
	})
	if err != nil {
		t.Fatalf("MaterializeInputProviders: %v", err)
	}
	if err := evaluationmod.AssertRegistryKeyParity(descs, evaluators, builders, providers); err != nil {
		t.Fatalf("AssertRegistryKeyParity: %v", err)
	}

	descKinds := descriptorDomainKinds(descs)
	for _, cap := range domain.ModelFamilyCapabilities() {
		if !cap.RuntimeExecutable {
			continue
		}
		if !descKinds[cap.Kind] {
			t.Fatalf("missing descriptor for runtime executable kind %q", cap.Kind)
		}
		path, err := evaldomain.ExecutionPathForDescriptor(descriptorForKind(descs, cap.Kind))
		if err != nil {
			t.Fatalf("ExecutionPathForDescriptor(%q): %v", cap.Kind, err)
		}
		if path != cap.ExecutionPath {
			t.Fatalf("kind %q execution path = %q, want %q", cap.Kind, path, cap.ExecutionPath)
		}
	}

	projectorKeys := make(map[evaldomain.ExecutionIdentity]bool, len(projectors))
	for _, projector := range projectors {
		projectorKeys[projector.Key()] = true
	}
	for _, desc := range descs {
		path, err := evaldomain.ExecutionPathForDescriptor(desc)
		if err != nil {
			t.Fatalf("ExecutionPathForDescriptor: %v", err)
		}
		switch path {
		case domain.ExecutionPathScaleDescriptor, domain.ExecutionPathBehavioralRatingDescriptor, domain.ExecutionPathCognitiveDescriptor:
			if !projectorKeys[desc.ExecutionIdentity()] {
				t.Fatalf("missing score projector for %s", desc.ExecutionIdentity())
			}
		}
	}
}

func descriptorForKind(descs []evaldomain.ModelDescriptor, kind domain.Kind) evaldomain.ModelDescriptor {
	for _, desc := range descs {
		if desc.ExecutionIdentity().Kind == kind {
			return desc
		}
	}
	return evaldomain.ModelDescriptor{}
}

type kindLandingNoopScoreRepo struct{}

func (kindLandingNoopScoreRepo) SaveScoresWithContext(context.Context, *assessment.Assessment, *assessment.ScaleScoreProjection) error {
	return nil
}
func (kindLandingNoopScoreRepo) DeleteByAssessmentID(context.Context, assessment.ID) error {
	return nil
}
