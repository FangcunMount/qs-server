package input

import (
	"context"
	"testing"
	"time"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
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

func TestFromOutcomeRecordPreservesDimensionlessSpecialTypologyFact(t *testing.T) {
	assets := &interpretationassets.Assets{
		Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "DRUNK", Title: "饮酒特殊结果", Summary: "冻结特殊结果摘要"}},
		Profiles: []interpretationassets.TypeProfilePresentation{{OutcomeCode: "DRUNK", Commentary: "冻结特殊结果摘要"}},
	}
	reportInput, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets: assets,
		ModelRef: evaluationinput.ModelRef{
			Kind:      evaluationinput.EvaluationModelKindTypology,
			Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "SBTI_FUN", Version: "v48",
		},
		DecisionKind: modelcatalog.DecisionKindNearestPattern,
		TypologyRouting: &evaluationinput.TypologyRoutingFreeze{
			DecisionKind: string(modelcatalog.DecisionKindNearestPattern), ReportKind: string(modeltypology.ReportKindPersonalityType),
			AdapterKey: string(modeltypology.ReportAdapterPersonalityType), TemplateID: "sbti", TemplateVersion: "legacy-v1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(30), OrgID: 1, AssessmentID: meta.FromUint64(31), TesteeID: 32, RunID: "31:1",
		Model: evaluationfact.ModelIdentity{
			Kind:      modelcatalog.KindTypology,
			Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "SBTI_FUN", Version: "v48",
		},
		Runtime: evaluationfact.RuntimeIdentity{
			DecisionKind: modelcatalog.DecisionKindNearestPattern,
		},
		SchemaVersion: 2, EvaluatedAt: time.Unix(200, 0), ReportInput: reportInput,
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"DRUNK","match_percent":100,"is_special":true,"special_trigger":"hidden:drink"}},"Primary":{"Kind":"match_percent","Value":100},"Level":{"Code":"DRUNK"},"Profile":{"Kind":"personality_type","Code":"DRUNK"}}`),
	})

	got, err := FromOutcomeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if got.PersonalityType == nil || !got.PersonalityType.Detail.IsSpecial || got.PersonalityType.Detail.SpecialTrigger != "hidden:drink" || got.PersonalityType.Detail.TypeCode != "DRUNK" {
		t.Fatalf("special personality detail = %#v", got.PersonalityType)
	}
	if len(got.PersonalityType.Detail.Dimensions) != 0 {
		t.Fatalf("dimensionless special fact gained dimensions: %#v", got.PersonalityType.Detail.Dimensions)
	}
}

func TestFromOutcomeRecordRestoresTraitProfileNamesFromFrozenFactorCatalog(t *testing.T) {
	assets := &interpretationassets.Assets{ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{
		Code: "trait_profile", Kind: "trait_profile", AdapterKey: "trait_profile", TemplateID: "enneagram",
	}}}}
	reportInput, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets: assets,
		ModelRef: evaluationinput.ModelRef{
			Kind:      evaluationinput.EvaluationModelKindTypology,
			Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "ENNEAGRAM_45", Version: "v16",
		},
		DecisionKind:  modelcatalog.DecisionKindTraitProfile,
		FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "type_1", Title: "完美型"}},
		TypologyRouting: &evaluationinput.TypologyRoutingFreeze{
			DecisionKind: string(modelcatalog.DecisionKindTraitProfile), ReportKind: string(modeltypology.ReportKindTraitProfile),
			AdapterKey: string(modeltypology.ReportAdapterTraitProfile), TemplateID: "enneagram",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(40), OrgID: 1, AssessmentID: meta.FromUint64(41), TesteeID: 42, RunID: "41:1",
		Model: evaluationfact.ModelIdentity{
			Kind:      modelcatalog.KindTypology,
			Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "ENNEAGRAM_45", Version: "v16",
		},
		Runtime: evaluationfact.RuntimeIdentity{
			DecisionKind: modelcatalog.DecisionKindTraitProfile,
		},
		SchemaVersion: 2, EvaluatedAt: time.Unix(300, 0), ReportInput: reportInput,
		Payload: []byte(`{"Dimensions":[{"Code":"type_1","Kind":"trait","Score":{"Kind":"raw_total","Value":8}}],"Profile":{"Kind":"personality_trait"}}`),
	})

	got, err := FromOutcomeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if got.TraitProfile == nil || len(got.TraitProfile.Detail.Traits) != 1 {
		t.Fatalf("trait profile = %#v", got.TraitProfile)
	}
	trait := got.TraitProfile.Detail.Traits[0]
	if trait.Code != "type_1" || trait.Name != "完美型" || trait.RawScore != 8 {
		t.Fatalf("trait = %#v", trait)
	}
	if got.Report.TemplateID != "enneagram" || got.Report.AdapterKey != string(modeltypology.ReportAdapterTraitProfile) {
		t.Fatalf("report routing = %#v", got.Report)
	}
}

func TestSPMSensoryReportInputHidesHelperFactorFromInterpretation(t *testing.T) {
	assets := &interpretationassets.Assets{
		Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "normal", Title: "与同龄儿童相似"}},
		ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{
			Code: "spm_sensory_scores", Kind: "factor_scores", SourceRefs: []string{"visible", "total"},
		}}},
	}
	modelRef := evaluationinput.ModelRef{
		Kind: evaluationinput.EvaluationModelKindBehavioralRating, Algorithm: string(modelcatalog.AlgorithmSPMSensory),
		Code: "bJFKi3", Version: "v16", Title: "SPM 感觉处理测量",
	}
	reportInput, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets: assets, ModelRef: modelRef, DecisionKind: modelcatalog.DecisionKindNormLookup,
		FactorCatalog: []evaluationinput.FactorCatalogEntry{
			{Code: "visible", Title: "社会参与"},
			{Code: "total", Title: "总分", IsTotalScore: true},
			{Code: "wcgKM7uV", Title: "味觉与嗅觉（仅计入 TOT）"},
		},
		Norming: &evaluationinput.NormingFreeze{NormTables: &calcnorm.NormTables{
			TScoreRules: []calcnorm.TScoreInterpretRule{
				{FactorCode: "visible", Ranges: []calcnorm.TScoreRange{{MinT: 0, MaxT: 100, Level: "normal", Conclusion: "正常"}}},
				{FactorCode: "total", Ranges: []calcnorm.TScoreRange{{MinT: 0, MaxT: 100, Level: "normal", Conclusion: "正常"}}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(50), OrgID: 1, AssessmentID: meta.FromUint64(51), TesteeID: 52, RunID: "51:1",
		Model: evaluationfact.ModelIdentity{
			Kind: modelcatalog.KindBehavioralRating, Algorithm: modelcatalog.AlgorithmSPMSensory,
			Code: "bJFKi3", Version: "v16", Title: "SPM 感觉处理测量",
		},
		Runtime: evaluationfact.RuntimeIdentity{
			DecisionKind: modelcatalog.DecisionKindNormLookup,
		},
		SchemaVersion: 2, EvaluatedAt: time.Unix(400, 0), ReportInput: reportInput,
		Payload: []byte(`{
			"Primary":{"Kind":"raw_total","Value":30},
			"Level":{"Code":"normal"},
			"Dimensions":[
				{"Code":"visible","Score":{"Kind":"raw_total","Value":10},"DerivedScores":[{"Kind":"t_score","Value":50}],"Level":{"Code":"normal"}},
				{"Code":"total","Role":"total","Score":{"Kind":"raw_total","Value":20},"DerivedScores":[{"Kind":"t_score","Value":50}],"Level":{"Code":"normal"}},
				{"Code":"wcgKM7uV","Score":{"Kind":"raw_total","Value":4},"DerivedScores":[{"Kind":"t_score","Value":50}],"Level":{"Code":"none"}}
			]
		}`),
	})

	input, err := FromOutcomeRecord(record)
	if err != nil {
		t.Fatalf("FromOutcomeRecord: %v", err)
	}
	if input.PresentationProfile == nil || !input.PresentationProfile.Configured() {
		t.Fatalf("presentation profile = %#v, want frozen visibility", input.PresentationProfile)
	}
	visible := input.PresentationProfile.VisibleSet()
	if !visible["visible"] || !visible["total"] || visible["wcgKM7uV"] {
		t.Fatalf("visible factors = %#v", visible)
	}

	reportBuilder := rendering.NewNormProfileBuilder(builder.NewDefaultReportBuilder())
	if _, err := reportBuilder.Build(context.Background(), input); err != nil {
		t.Fatalf("hidden helper factor must not require outcome presentation: %v", err)
	}
}

func TestFromOutcomeRecordRejectsNonCurrentReportInput(t *testing.T) {
	record := typologyOutcomeRecord([]byte(`{"schema_version":2}`))
	if _, err := FromOutcomeRecord(record); err == nil {
		t.Fatal("non-current report input was accepted")
	}
}

func TestFromOutcomeRecordRejectsMissingReportInputForFactorScoring(t *testing.T) {
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID:           meta.FromUint64(60),
		OrgID:        1,
		AssessmentID: meta.FromUint64(61),
		TesteeID:     62,
		RunID:        "61:1",
		Model: evaluationfact.ModelIdentity{
			Kind:      modelcatalog.KindScale,
			Algorithm: modelcatalog.AlgorithmScaleDefault,
			Code:      "SCALE-1",
			Version:   "1.0.0",
		},
		Runtime: evaluationfact.RuntimeIdentity{
			DecisionKind: modelcatalog.DecisionKindScoreRange,
		},
		SchemaVersion: 2,
		EvaluatedAt:   time.Unix(500, 0),
		Payload: []byte(`{
			"Primary":{"Kind":"raw_total","Value":10},
			"Dimensions":[
				{"Code":"total","Role":"total","Score":{"Kind":"raw_total","Value":10},"Level":{"Code":"normal"}}
			]
		}`),
	})

	if _, err := FromOutcomeRecord(record); err == nil {
		t.Fatal("factor-scoring outcome without ReportInput was accepted")
	}
}

func TestFromOutcomeRecordRejectsMissingFrozenTypologyRouting(t *testing.T) {
	assets := &interpretationassets.Assets{Profiles: []interpretationassets.TypeProfilePresentation{{OutcomeCode: "INTJ", Commentary: "冻结摘要"}}}
	_, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets:       assets,
		ModelRef:     evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "PERSONALITY", Version: "1.0.0"},
		DecisionKind: modelcatalog.DecisionKindPoleComposition,
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
		ModelRef:        evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "PERSONALITY", Version: "1.0.0"},
		DecisionKind:    modelcatalog.DecisionKindPoleComposition,
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
			Kind:      modelcatalog.KindTypology,
			Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "PERSONALITY", Version: "1.0.0",
		},
		Runtime: evaluationfact.RuntimeIdentity{
			DecisionKind: modelcatalog.DecisionKindPoleComposition,
		},
		SchemaVersion: 2, EvaluatedAt: time.Unix(100, 0), ReportInput: reportInput,
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80}},"Primary":{"Kind":"match_percent","Value":80,"Label":"INTJ"},"Profile":{"Kind":"personality_type","Code":"INTJ"}}`),
	})
}
