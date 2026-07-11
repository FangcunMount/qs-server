package reporting

import (
	"context"
	"testing"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationruntime"
)

type registryReportBuilderStub struct {
	key       evaluation.ExecutionIdentity
	mechanism MechanismReportBuilderKey
}

func (b registryReportBuilderStub) ExecutionIdentity() evaluation.ExecutionIdentity { return b.key }
func (b registryReportBuilderStub) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b registryReportBuilderStub) Key() evaluation.ExecutionIdentity {
	return b.ExecutionIdentity()
}
func (registryReportBuilderStub) TemplateVersion() policy.TemplateVersion {
	return policy.TemplateVersionV1
}
func (registryReportBuilderStub) BuilderIdentity() string      { return "registry-test" }
func (registryReportBuilderStub) ContentSchemaVersion() string { return "report-content/v1" }
func (b registryReportBuilderStub) Build(context.Context, interpinput.InterpretationInput) (*report.Draft, error) {
	return nil, nil
}
func (b registryReportBuilderStub) MechanismKey() MechanismReportBuilderKey {
	if b.mechanism.AlgorithmFamily != "" {
		return b.mechanism
	}
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
}

func TestReportBuilderRegistryRejectsDuplicateKey(t *testing.T) {
	_, err := NewReportBuilderRegistry(
		registryReportBuilderStub{key: evaluation.ExecutionIdentityScaleDefault},
		registryReportBuilderStub{key: evaluation.ExecutionIdentityScaleDefault},
	)
	if err == nil {
		t.Fatal("NewReportBuilderRegistry error = nil, want duplicate key")
	}
}

func TestReportBuilderRegistryRejectsUnknownKey(t *testing.T) {
	registry, err := NewReportBuilderRegistry(registryReportBuilderStub{key: evaluation.ExecutionIdentityScaleDefault})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	if _, err := registry.Resolve(evaluation.PersonalityTypologyIdentity(modelcatalog.AlgorithmMBTI), domainReport.ReportTypeStandard); err == nil {
		t.Fatal("Resolve error = nil, want unsupported key")
	}
}

func TestReportBuilderRegistryResolvesLegacyTypologyViaConfiguredKey(t *testing.T) {
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

func legacyTypologyIdentities() []evaluation.ExecutionIdentity {
	return []evaluation.ExecutionIdentity{
		evaluation.PersonalityTypologyIdentity(modelcatalog.AlgorithmMBTI),
		evaluation.PersonalityTypologyIdentity(modelcatalog.AlgorithmSBTI),
		evaluation.PersonalityTypologyIdentity(modelcatalog.AlgorithmBigFive),
	}
}

func TestReportBuilderRegistryResolvesByMechanismKey(t *testing.T) {
	registry, err := NewReportBuilderRegistry(NewFactorScoringReportBuilder(nil))
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	builder, err := registry.ResolveByMechanism(MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	})
	if err != nil {
		t.Fatal(err)
	}
	if builder.Key() != evaluation.ExecutionIdentityScaleDefault {
		t.Fatalf("builder key = %s", builder.Key())
	}
}

func TestReportBuilderRegistryFallsBackToMechanismFromEvaluatorKey(t *testing.T) {
	registry, err := NewReportBuilderRegistry(NewNormProfileReportBuilder(nil))
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	builder, err := registry.Resolve(evaluation.ExecutionIdentityBehavioralRatingDefault, domainReport.ReportTypeStandard)
	if err != nil {
		t.Fatal(err)
	}
	if builder.Key() != evaluation.ExecutionIdentityBehavioralRatingDefault {
		t.Fatalf("builder key = %s", builder.Key())
	}
}
