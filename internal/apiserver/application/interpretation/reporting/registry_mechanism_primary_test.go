package reporting

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestResolvePrefersMechanismKeyOverEvaluatorKey(t *testing.T) {
	t.Parallel()

	registry, err := NewReportBuilderRegistry(NewFactorScoringReportBuilder(nil))
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	mechanismKey := MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
	byMechanism, err := registry.ResolveByMechanism(mechanismKey)
	if err != nil {
		t.Fatal(err)
	}
	byEvaluator, err := registry.Resolve(evaluation.EvaluatorKeyScaleDefault, domainReport.ReportTypeStandard)
	if err != nil {
		t.Fatal(err)
	}
	if byMechanism.Key() != byEvaluator.Key() {
		t.Fatalf("mechanism key=%s evaluator key=%s", byMechanism.Key(), byEvaluator.Key())
	}
}

func TestResolveUsesMechanismForNormProfileBuilder(t *testing.T) {
	t.Parallel()

	registry, err := NewReportBuilderRegistry(NewNormProfileReportBuilder(nil))
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	builder, err := registry.Resolve(evaluation.EvaluatorKeyBehavioralRatingDefault, domainReport.ReportTypeStandard)
	if err != nil {
		t.Fatal(err)
	}
	mechanismKey, ok := builder.(MechanismKeyedReportBuilder)
	if !ok {
		t.Fatal("builder does not implement MechanismKeyedReportBuilder")
	}
	if mechanismKey.MechanismKey().AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorNorm {
		t.Fatalf("family=%s", mechanismKey.MechanismKey().AlgorithmFamily)
	}
}

func TestResolveLegacyTypologyStillWorksAfterMechanismPrimary(t *testing.T) {
	t.Parallel()

	registry, err := NewReportBuilderRegistry(registryReportBuilderStub{
		key: evaluation.EvaluatorKeyPersonalityTypology,
	})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	for _, legacyKey := range evaluation.PersonalityTypologyLegacyKeys() {
		builder, err := registry.Resolve(legacyKey, domainReport.ReportTypeStandard)
		if err != nil {
			t.Fatalf("Resolve(%s): %v", legacyKey, err)
		}
		if builder.Key() != evaluation.EvaluatorKeyPersonalityTypology {
			t.Fatalf("builder key = %s, want configured typology", builder.Key())
		}
	}
}

func TestMechanismReportBuilderKeyFromOutcomeMatchesEvaluatorDerivation(t *testing.T) {
	t.Parallel()

	key, ok := MechanismReportBuilderKeyFromEvaluatorKey(evaluation.EvaluatorKeyScaleDefault, domainReport.ReportTypeStandard)
	if !ok {
		t.Fatal("MechanismReportBuilderKeyFromEvaluatorKey returned false")
	}
	if key.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorScoring {
		t.Fatalf("family=%s", key.AlgorithmFamily)
	}
	if key.DecisionKind != modelcatalog.DecisionKindScoreRange {
		t.Fatalf("decision=%s", key.DecisionKind)
	}
}
