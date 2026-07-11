package reporting

import (
	"testing"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationruntime"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
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
	byEvaluator, err := registry.Resolve(evaluation.ExecutionIdentityScaleDefault, domainReport.ReportTypeStandard)
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
	builder, err := registry.Resolve(evaluation.ExecutionIdentityBehavioralRatingDefault, domainReport.ReportTypeStandard)
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
		key: evaluation.ExecutionIdentityPersonalityTypology,
		mechanism: MechanismReportBuilderKey{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindPoleComposition,
			ReportType:      domainReport.ReportTypeStandard,
		},
	})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	for _, legacyKey := range legacyTypologyIdentities() {
		builder, err := registry.Resolve(legacyKey, domainReport.ReportTypeStandard)
		if err != nil {
			t.Fatalf("Resolve(%s): %v", legacyKey, err)
		}
		if builder.Key() != evaluation.ExecutionIdentityPersonalityTypology {
			t.Fatalf("builder key = %s, want configured typology", builder.Key())
		}
	}
}

func TestMechanismReportBuilderKeyFromOutcomeUsesInputSnapshot(t *testing.T) {
	t.Parallel()

	outcome := evaloutcome.Outcome{
		Input: &evaluationinput.InputSnapshot{
			Model: &evaluationinput.ModelSnapshot{
				Kind:      evaluationinput.EvaluationModelKindScale,
				Algorithm: "scale_default",
				Code:      "PHQ9",
			},
		},
	}
	key, ok := MechanismReportBuilderKeyFromOutcome(outcome)
	if !ok {
		t.Fatal("MechanismReportBuilderKeyFromOutcome returned false")
	}
	if key.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorScoring {
		t.Fatalf("family=%s", key.AlgorithmFamily)
	}
	if key.DecisionKind != modelcatalog.DecisionKindScoreRange {
		t.Fatalf("decision=%s", key.DecisionKind)
	}
}

func TestReportRoutingContextFromOutcomeCarriesAlgorithmAndProductChannel(t *testing.T) {
	t.Parallel()

	outcome := evaloutcome.Outcome{
		Input: &evaluationinput.InputSnapshot{
			Model: &evaluationinput.ModelSnapshot{
				Kind:           evaluationinput.EvaluationModelKindScale,
				Algorithm:      string(modelcatalog.AlgorithmScaleDefault),
				ProductChannel: string(modelcatalog.ProductChannel("screening")),
				Code:           "PHQ9",
			},
		},
	}
	ctx, ok := ReportRoutingContextFromOutcome(outcome)
	if !ok {
		t.Fatal("ReportRoutingContextFromOutcome returned false")
	}
	if ctx.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorScoring {
		t.Fatalf("family=%s", ctx.AlgorithmFamily)
	}
	if ctx.DecisionKind != modelcatalog.DecisionKindScoreRange {
		t.Fatalf("decision=%s", ctx.DecisionKind)
	}
	if ctx.Algorithm != modelcatalog.AlgorithmScaleDefault {
		t.Fatalf("algorithm=%s", ctx.Algorithm)
	}
	if ctx.ProductChannel != modelcatalog.ProductChannel("screening") {
		t.Fatalf("product channel=%s", ctx.ProductChannel)
	}
	key, ok := ctx.MechanismKey()
	if !ok {
		t.Fatal("MechanismKey returned false")
	}
	if key.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorScoring {
		t.Fatalf("mechanism family=%s", key.AlgorithmFamily)
	}
	if key.DecisionKind != modelcatalog.DecisionKindScoreRange {
		t.Fatalf("mechanism decision=%s", key.DecisionKind)
	}
	if key.ReportType != domainReport.ReportTypeStandard {
		t.Fatalf("report type=%s", key.ReportType)
	}
}

func TestMechanismReportBuilderKeyFromOutcomeUsesBehavioralRatingExecutionFamily(t *testing.T) {
	t.Parallel()

	outcome := evaloutcome.Outcome{
		Input: behavioralRatingInputSnapshotForMechanismKey(t),
	}
	key, ok := MechanismReportBuilderKeyFromOutcome(outcome)
	if !ok {
		t.Fatal("MechanismReportBuilderKeyFromOutcome returned false")
	}
	if key.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorNorm {
		t.Fatalf("family=%s", key.AlgorithmFamily)
	}
	if key.DecisionKind != modelcatalog.DecisionKindNormLookup {
		t.Fatalf("decision=%s", key.DecisionKind)
	}
}

func behavioralRatingInputSnapshotForMechanismKey(t *testing.T) *evaluationinput.InputSnapshot {
	t.Helper()
	snapshot := &behavioralsnapshot.Snapshot{
		Code:    "BR-001",
		Version: "1.0.0",
		Title:   "行为评分",
	}
	return &evaluationinput.InputSnapshot{
		Model: evaluationinput.NewBehavioralRatingModelSnapshot(snapshot),
	}
}

func TestEventAssemblerResolveByMechanism(t *testing.T) {
	t.Parallel()

	registry, err := NewEventAssemblerRegistry(DefaultMechanismEventAssemblers()...)
	if err != nil {
		t.Fatalf("NewEventAssemblerRegistry: %v", err)
	}
	mechanismKey := MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
		DecisionKind:    modelcatalog.DecisionKindPoleComposition,
		ReportType:      domainReport.ReportTypeStandard,
	}
	assembler := registry.ResolveByMechanism(mechanismKey)
	if assembler.Key() != evaluation.ExecutionIdentityPersonalityTypology {
		t.Fatalf("assembler key=%s", assembler.Key())
	}
}
