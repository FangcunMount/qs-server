package input

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestFromOutcomeRecordUsesCurrentFrozenTypologyInput(t *testing.T) {
	reportInput := currentTypologyReportInput(t, &evaluationinput.TypologyRoutingFreeze{
		DecisionKind: string(modelcatalog.DecisionKindPoleComposition),
		ReportKind:   string(modeltypology.ReportKindTemplate), AdapterKey: string(modeltypology.ReportAdapterPersonalityType),
		TemplateID: "personality", TemplateVersion: "custom-v3",
	})
	record := typologyOutcomeRecord(reportInput)

	got, err := FromOutcomeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	detail := got.PersonalityType.Detail
	if detail.TypeCode != "INTJ" || detail.TypeName != "建筑师" || detail.Profile.Summary != "冻结摘要" {
		t.Fatalf("personality detail = %#v", detail)
	}
	if got.Report.TemplateID != "personality" || got.Report.AdapterKey != string(modeltypology.ReportAdapterPersonalityType) {
		t.Fatalf("report routing = %#v", got.Report)
	}
	if got.Report.TemplateVersion.String() != "custom-v3" {
		t.Fatalf("template version = %q", got.Report.TemplateVersion)
	}
}

func TestFromOutcomeRecordRejectsNonCurrentReportInput(t *testing.T) {
	record := typologyOutcomeRecord([]byte(`{"schema_version":2}`))
	if _, err := FromOutcomeRecord(record); err == nil {
		t.Fatal("non-current report input was accepted")
	}
}

func TestFromOutcomeRecordRejectsMissingFrozenTypologyRouting(t *testing.T) {
	assets := &interpretationassets.Assets{Profiles: []interpretationassets.TypeProfilePresentation{{OutcomeCode: "INTJ", Commentary: "冻结摘要"}}}
	_, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets:          assets,
		ModelRef:        evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "PERSONALITY", Version: "1.0.0"},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
	})
	if err == nil {
		t.Fatal("typology report input without routing was frozen")
	}
}

func currentTypologyReportInput(t *testing.T, routing *evaluationinput.TypologyRoutingFreeze) []byte {
	t.Helper()
	assets := &interpretationassets.Assets{
		Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "INTJ", Title: "建筑师", Summary: "冻结摘要"}},
		Profiles: []interpretationassets.TypeProfilePresentation{{OutcomeCode: "INTJ", Commentary: "冻结摘要", Strengths: []string{"系统思考"}}},
	}
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets:          assets,
		ModelRef:        evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, SubKind: string(modelcatalog.SubKindTypology), Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "PERSONALITY", Version: "1.0.0"},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
		TypologyRouting: routing,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func typologyOutcomeRecord(reportInput []byte) *evaluationfact.Record {
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(20), OrgID: 1, AssessmentID: meta.FromUint64(10), TesteeID: 2, RunID: "10:1",
		Model: evaluationfact.ModelIdentity{
			Kind: modelcatalog.KindTypology, SubKind: modelcatalog.SubKindTypology,
			Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "PERSONALITY", Version: "1.0.0",
		},
		Runtime: evaluationfact.RuntimeIdentity{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindPoleComposition,
		},
		SchemaVersion: 2, EvaluatedAt: time.Unix(100, 0), ReportInput: reportInput,
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80}},"Primary":{"Kind":"match_percent","Value":80,"Label":"INTJ"},"Profile":{"Kind":"personality_type","Code":"INTJ"}}`),
	})
}
