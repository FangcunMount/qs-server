package result

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type registryReportBuilderStub struct {
	key evaluation.EvaluatorKey
}

func (b registryReportBuilderStub) Key() evaluation.EvaluatorKey { return b.key }
func (registryReportBuilderStub) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}
func (b registryReportBuilderStub) Build(context.Context, Outcome) (*domainReport.InterpretReport, error) {
	return nil, nil
}

func TestReportBuilderRegistryRejectsDuplicateKey(t *testing.T) {
	_, err := NewReportBuilderRegistry(
		registryReportBuilderStub{key: evaluation.EvaluatorKeyScaleDefault},
		registryReportBuilderStub{key: evaluation.EvaluatorKeyScaleDefault},
	)
	if err == nil {
		t.Fatal("NewReportBuilderRegistry error = nil, want duplicate key")
	}
}

func TestReportBuilderRegistryRejectsUnknownKey(t *testing.T) {
	registry, err := NewReportBuilderRegistry(registryReportBuilderStub{key: evaluation.EvaluatorKeyScaleDefault})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	if _, err := registry.Resolve(evaluation.EvaluatorKeyMBTI, domainReport.ReportTypeStandard); err == nil {
		t.Fatal("Resolve error = nil, want unsupported key")
	}
}

func TestReportBuilderRegistryResolvesLegacyTypologyViaConfiguredKey(t *testing.T) {
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
