package input

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestFromOutcomeRecordSchemaV2ResolvesReportProseFromFrozenInput(t *testing.T) {
	assets, err := json.Marshal(evaluationinput.TypologyModelPayload{Payload: &modeltypology.Payload{
		Code: "MBTI", Version: "1", Algorithm: modelcatalog.AlgorithmMBTI,
		Source: modeltypology.Source{Attribution: "frozen-source"},
		Outcomes: []modeltypology.Outcome{{
			Code: "INTJ", Name: "建筑师", OneLiner: "独立战略家", Summary: "冻结摘要",
			Strengths: []string{"系统思考"}, Suggestions: []string{"保留沟通空间"},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(20), OrgID: 1, AssessmentID: meta.FromUint64(10), TesteeID: 2, RunID: "10:1",
		Model:         evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, SubKind: modelcatalog.SubKindTypology, Algorithm: modelcatalog.AlgorithmMBTI, Code: "MBTI", Version: "1"},
		Runtime:       evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		SchemaVersion: 2, EvaluatedAt: time.Unix(100, 0), ReportInput: assets,
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80}},"Primary":{"Kind":"match_percent","Value":80,"Label":"INTJ"},"Profile":{"Kind":"personality_type","Code":"INTJ"}}`),
	})

	got, err := FromOutcomeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	detail := got.PersonalityType.Detail
	if detail.TypeCode != "INTJ" || detail.TypeName != "建筑师" || detail.OneLiner != "独立战略家" || detail.Profile.Summary != "冻结摘要" {
		t.Fatalf("v2 report detail = %#v", detail)
	}
}

func TestFromOutcomeRecordSchemaV2RejectsCodeMissingFromFrozenInput(t *testing.T) {
	assets, _ := json.Marshal(evaluationinput.TypologyModelPayload{Payload: &modeltypology.Payload{Code: "MBTI", Outcomes: []modeltypology.Outcome{{Code: "ENFP"}}}})
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(21), AssessmentID: meta.FromUint64(11), TesteeID: 2, RunID: "11:1",
		Model:         evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, Code: "MBTI"},
		Runtime:       evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		SchemaVersion: 2, EvaluatedAt: time.Unix(100, 0), ReportInput: assets,
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ"}}}`),
	})
	if _, err := FromOutcomeRecord(record); err == nil {
		t.Fatal("expected frozen report input lookup failure")
	}
}

func TestFromOutcomeRecordReportRouting_LegacyAbsentAllowsDerive(t *testing.T) {
	assets, err := json.Marshal(evaluationinput.TypologyModelPayload{Payload: &modeltypology.Payload{
		Code: "MBTI", Version: "1", Algorithm: modelcatalog.AlgorithmMBTI,
		DimensionOrder: []string{"EI"},
		Dimensions:     map[string]modeltypology.Dimension{"EI": {Code: "EI", Name: "EI", LeftPole: "I", RightPole: "E"}},
		MatchingSpec:   modeltypology.MatchingSpec{Kind: modelcatalog.DecisionKindPoleComposition},
		Outcomes:       []modeltypology.Outcome{{Code: "INTJ", Name: "建筑师"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(30), AssessmentID: meta.FromUint64(30), TesteeID: 2, RunID: "30:1",
		Model:         evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmMBTI, Code: "MBTI", Version: "1"},
		Runtime:       evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		SchemaVersion: 2, EvaluatedAt: time.Unix(100, 0), ReportInput: assets,
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80}},"Primary":{"Kind":"match_percent","Value":80,"Label":"INTJ"}}`),
	})

	got, err := FromOutcomeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if got.Report.TemplateID != "mbti" {
		t.Fatalf("TemplateID = %q, want mbti from legacy derive", got.Report.TemplateID)
	}
	if got.Report.AdapterKey != string(modeltypology.ReportAdapterPersonalityType) {
		t.Fatalf("AdapterKey = %q, want personality_type", got.Report.AdapterKey)
	}
}

func TestFromOutcomeRecordReportRouting_ExplicitValid(t *testing.T) {
	assets, err := json.Marshal(evaluationinput.TypologyModelPayload{Payload: &modeltypology.Payload{
		Code: "MBTI", Version: "1",
		Outcomes: []modeltypology.Outcome{{Code: "INTJ", Name: "建筑师"}},
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				DimensionOrder: []string{"EI"},
				Dimensions:     map[string]modeltypology.Dimension{"EI": {Code: "EI", Name: "EI", LeftPole: "I", RightPole: "E"}},
			},
			Decision:       modeltypology.PersonalityDecisionSpec{Kind: modelcatalog.DecisionKindPoleComposition},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{DetailKind: modeltypology.OutcomeDetailPersonalityType},
			Report: modeltypology.ReportSpec{
				Kind:       modeltypology.ReportKindTemplate,
				AdapterKey: modeltypology.ReportAdapterPersonalityType,
				TemplateID: "mbti",
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(31), AssessmentID: meta.FromUint64(31), TesteeID: 2, RunID: "31:1",
		Model:         evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmMBTI, Code: "MBTI", Version: "1"},
		Runtime:       evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		SchemaVersion: 2, EvaluatedAt: time.Unix(100, 0), ReportInput: assets,
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80}},"Primary":{"Kind":"match_percent","Value":80,"Label":"INTJ"}}`),
	})

	got, err := FromOutcomeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if got.Report.TemplateID != "mbti" || got.Report.AdapterKey != string(modeltypology.ReportAdapterPersonalityType) {
		t.Fatalf("report routing = TemplateID=%q AdapterKey=%q", got.Report.TemplateID, got.Report.AdapterKey)
	}
}

func TestFromOutcomeRecordReportRouting_ExplicitMalformedFailClosed(t *testing.T) {
	assets, err := json.Marshal(evaluationinput.TypologyModelPayload{Payload: &modeltypology.Payload{
		Code: "MBTI", Version: "1",
		Outcomes: []modeltypology.Outcome{{Code: "INTJ", Name: "建筑师"}},
		Runtime: &modeltypology.RuntimeSpec{
			Report: modeltypology.ReportSpec{Kind: modeltypology.ReportKindTemplate},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(32), AssessmentID: meta.FromUint64(32), TesteeID: 2, RunID: "32:1",
		Model:         evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmMBTI, Code: "MBTI", Version: "1"},
		Runtime:       evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		SchemaVersion: 2, EvaluatedAt: time.Unix(100, 0), ReportInput: assets,
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80}},"Primary":{"Kind":"match_percent","Value":80,"Label":"INTJ"}}`),
	})

	_, err = FromOutcomeRecord(record)
	if err == nil || !errors.Is(err, modeltypology.ErrRuntimeSpecInvalid) {
		t.Fatalf("err = %v, want ErrRuntimeSpecInvalid", err)
	}
}
