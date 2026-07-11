package modelcatalog

import (
	"testing"

	reportmaterialize "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/materialize"
	evaluationmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
)

// Kind landing contract: every RuntimeExecutable capability must materialize evaluator/builder/provider.
func TestRuntimeExecutableKindsSatisfyLandingContract(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	builders, err := reportmaterialize.ReportBuilders(descs, report.NewDefaultReportBuilder(nil))
	if err != nil {
		t.Fatalf("ReportBuilders: %v", err)
	}
	if len(builders) != len(descs) {
		t.Fatalf("report builder count = %d, want %d", len(builders), len(descs))
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
	if err := evaluationmod.AssertExecutionPathParity(descs, providers); err != nil {
		t.Fatalf("AssertExecutionPathParity: %v", err)
	}

	descKinds := descriptorDomainKinds(descs)
	for _, kind := range domain.RuntimeExecutableKinds() {
		cap, ok := domain.FamilyCapabilityByKind(kind)
		if !ok || !cap.RuntimeExecutable {
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

}

func descriptorForKind(descs []evaldomain.ModelDescriptor, kind domain.Kind) evaldomain.ModelDescriptor {
	for _, desc := range descs {
		if desc.ExecutionIdentity().Kind == kind {
			return desc
		}
	}
	return evaldomain.ModelDescriptor{}
}
