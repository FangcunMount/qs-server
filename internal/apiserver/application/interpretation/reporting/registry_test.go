package reporting

import (
	"context"
	"testing"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type registryReportBuilderStub struct {
	mechanism MechanismReportBuilderKey
}

func (b registryReportBuilderStub) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
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
		registryReportBuilderStub{},
		registryReportBuilderStub{},
	)
	if err == nil {
		t.Fatal("NewReportBuilderRegistry error = nil, want duplicate key")
	}
}

func TestReportBuilderRegistryRejectsUnknownKey(t *testing.T) {
	registry, err := NewReportBuilderRegistry(registryReportBuilderStub{})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	if _, err := registry.ResolveByMechanism(MechanismReportBuilderKey{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition, ReportType: domainReport.ReportTypeStandard}); err == nil {
		t.Fatal("ResolveByMechanism error = nil, want unsupported key")
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
	keyed, ok := builder.(MechanismKeyedReportBuilder)
	if !ok || keyed.MechanismKey().AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorScoring {
		t.Fatalf("builder mechanism = %#v", builder)
	}
}
