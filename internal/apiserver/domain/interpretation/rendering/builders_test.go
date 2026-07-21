package rendering_test

import (
	"context"
	"errors"
	"testing"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestTypologyBuilderUnknownTemplateIDFailClosed(t *testing.T) {
	t.Parallel()

	builder := rendering.NewTypologyBuilder()
	_, err := builder.Build(context.Background(), interpinput.InterpretationInput{
		Runtime: interpinput.RuntimeIdentity{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindPoleComposition,
		},
		Report: interpinput.ReportSpec{
			ReportType: policy.ReportTypeStandard,
			TemplateID: "not-registered",
			AdapterKey: string(reporttypology.ReportAdapterPersonalityType),
		},
		PersonalityType: &interpinput.PersonalityTypeFacts{
			Detail: reporttypology.PersonalityTypeReportDetail{TypeCode: "INTJ", TypeName: "建筑师"},
		},
	})
	if !errors.Is(err, reporttypology.ErrUnknownTemplateID) {
		t.Fatalf("err = %v, want ErrUnknownTemplateID", err)
	}
}
