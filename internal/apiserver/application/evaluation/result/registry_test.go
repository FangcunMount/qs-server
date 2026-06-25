package result

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

type registryReportBuilderStub struct {
	kind assessment.EvaluationModelKind
}

func (b registryReportBuilderStub) Kind() assessment.EvaluationModelKind { return b.kind }
func (registryReportBuilderStub) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}
func (b registryReportBuilderStub) Build(context.Context, Outcome) (*domainReport.InterpretReport, error) {
	return nil, nil
}

func TestReportBuilderRegistryRejectsDuplicateKind(t *testing.T) {
	_, err := NewReportBuilderRegistry(
		registryReportBuilderStub{kind: assessment.EvaluationModelKindScale},
		registryReportBuilderStub{kind: assessment.EvaluationModelKindScale},
	)
	if err == nil {
		t.Fatal("NewReportBuilderRegistry error = nil, want duplicate kind")
	}
}

func TestReportBuilderRegistryRejectsUnknownKind(t *testing.T) {
	registry, err := NewReportBuilderRegistry(registryReportBuilderStub{kind: assessment.EvaluationModelKindScale})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	if _, err := registry.Resolve(assessment.EvaluationModelKindMBTI, domainReport.ReportTypeStandard); err == nil {
		t.Fatal("Resolve error = nil, want unsupported kind")
	}
}
