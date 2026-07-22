package rendering_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestNormProfileBuilderCarriesFrozenVisibilityWithoutInterpretingHiddenFactor(t *testing.T) {
	t.Parallel()

	profile := report.NewFrozenPresentationProfile([]string{"visible"})
	reportBuilder := rendering.NewNormProfileBuilder(builder.NewDefaultReportBuilder())
	draft, err := reportBuilder.Build(context.Background(), interpinput.InterpretationInput{
		Association:         report.Association{AssessmentID: meta.FromUint64(201)},
		PresentationProfile: &profile,
		FactorScoring: &interpinput.FactorScoringFacts{
			Model: &reportscore.ReportModel{Factors: []reportscore.FactorReportModel{
				{Code: "visible", Title: "可见因子"},
				{Code: "hidden", Title: "隐藏因子"},
			}},
			Factors: []reportscore.FactorReportScore{
				{FactorCode: "visible", RawScore: 10, Conclusion: "正常", Suggestion: "保持"},
				{FactorCode: "hidden", RawScore: 12},
			},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	content := draft.Content()
	if len(content.Dimensions) != 2 {
		t.Fatalf("len(Dimensions) = %d, want hidden facts retained", len(content.Dimensions))
	}
	if content.PresentationProfile == nil || !content.PresentationProfile.Configured() || len(content.PresentationProfile.VisibleFactorCodes) != 1 || content.PresentationProfile.VisibleFactorCodes[0] != "visible" {
		t.Fatalf("presentation profile = %#v", content.PresentationProfile)
	}
}

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

func TestSBTISpecialOutcomeBuildsArtifactWithoutSyntheticDimensions(t *testing.T) {
	t.Parallel()

	reportBuilder := rendering.NewTypologyBuilder()
	input := interpinput.InterpretationInput{
		OutcomeID:   meta.FromUint64(301),
		Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(302), TesteeID: 303},
		Model: report.ModelIdentity{
			Kind: string(modelcatalog.KindTypology), SubKind: string(modelcatalog.SubKindTypology), Algorithm: string(modelcatalog.AlgorithmPersonalityTypology),
			Code: "SBTI_FUN", Version: "v48", Title: "SBTI 趣味人格测评", AlgorithmFamily: string(modelcatalog.AlgorithmFamilyFactorClassification),
		},
		Runtime: interpinput.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindNearestPattern},
		Result:  interpinput.ResultFacts{Primary: report.NewMatchPercentScore(100, "100%"), Level: &report.ResultLevel{Code: "DRUNK", Label: "饮酒特殊结果"}},
		Report: interpinput.ReportSpec{
			ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
			TemplateID: "sbti", AdapterKey: string(reporttypology.ReportAdapterPersonalityType),
		},
		PersonalityType: &interpinput.PersonalityTypeFacts{Detail: reporttypology.PersonalityTypeReportDetail{
			TypeCode: "DRUNK", TypeName: "饮酒特殊结果", OneLiner: "特殊结果",
			MatchPercent: 100, IsSpecial: true, SpecialTrigger: "hidden:drink",
		}},
	}

	draft, err := reportBuilder.Build(context.Background(), input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	artifact, err := report.NewInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(304), GenerationID: meta.FromUint64(305), OutcomeID: input.OutcomeID, InterpretationRunID: meta.FromUint64(306),
		Association: input.Association, ReportType: input.Report.ReportType, TemplateVersion: input.Report.TemplateVersion,
		BuilderIdentity: reportBuilder.BuilderIdentity(), ContentSchemaVersion: reportBuilder.ContentSchemaVersion(),
		Content: draft.Content(), GeneratedAt: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NewInterpretReport: %v", err)
	}
	content := artifact.Content()
	if len(content.Dimensions) != 0 {
		t.Fatalf("special outcome dimensions = %#v, want no synthetic dimensions", content.Dimensions)
	}
	if content.ModelExtra == nil || !content.ModelExtra.IsSpecial || content.ModelExtra.SpecialTrigger != "hidden:drink" || content.ModelExtra.TypeCode != "DRUNK" {
		t.Fatalf("special outcome model extra = %#v", content.ModelExtra)
	}
}

func TestTypologyTemplatesBuildCanonicalArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		templateID   string
		adapterKey   reporttypology.ReportAdapterKey
		decisionKind modelcatalog.DecisionKind
		wantKind     string
		facts        func(*interpinput.InterpretationInput)
	}{
		{
			name: "mbti", templateID: "mbti", adapterKey: reporttypology.ReportAdapterPersonalityType,
			decisionKind: modelcatalog.DecisionKindPoleComposition, wantKind: string(reporttypology.ReportAdapterPersonalityType),
			facts: func(input *interpinput.InterpretationInput) {
				input.PersonalityType = &interpinput.PersonalityTypeFacts{Detail: reporttypology.PersonalityTypeReportDetail{
					TypeCode: "INTJ", TypeName: "建筑师", OneLiner: "理性而坚定",
					Dimensions: []reporttypology.PersonalityTypeDimensionReport{{Code: "EI", Name: "精力倾向", RawScore: 26, Preference: "I", Strength: 65}},
				}}
			},
		},
		{
			name: "sbti", templateID: "sbti", adapterKey: reporttypology.ReportAdapterPersonalityType,
			decisionKind: modelcatalog.DecisionKindNearestPattern, wantKind: string(reporttypology.ReportAdapterPersonalityType),
			facts: func(input *interpinput.InterpretationInput) {
				input.PersonalityType = &interpinput.PersonalityTypeFacts{Detail: reporttypology.PersonalityTypeReportDetail{
					TypeCode: "SAGE", TypeName: "智者", OneLiner: "善于洞察",
					Dimensions: []reporttypology.PersonalityTypeDimensionReport{{Code: "thinking", Name: "思考", RawScore: 5, Level: "high", Model: "SAGE"}},
				}}
			},
		},
		{
			name: "bigfive", templateID: "bigfive", adapterKey: reporttypology.ReportAdapterTraitProfile,
			decisionKind: modelcatalog.DecisionKindTraitProfile, wantKind: string(reporttypology.ReportAdapterTraitProfile),
			facts: func(input *interpinput.InterpretationInput) {
				input.TraitProfile = &interpinput.TraitProfileFacts{Detail: reporttypology.TraitProfileReportDetail{
					Traits: []reporttypology.TraitProfileFactorReport{{Code: "O", Name: "开放性", RawScore: 38}},
				}}
			},
		},
		{
			name: "enneagram", templateID: "enneagram", adapterKey: reporttypology.ReportAdapterTraitProfile,
			decisionKind: modelcatalog.DecisionKindTraitProfile, wantKind: string(reporttypology.ReportAdapterTraitProfile),
			facts: func(input *interpinput.InterpretationInput) {
				input.Model.Code, input.Model.Title = "ENNEAGRAM_45", "九型人格"
				input.TraitProfile = &interpinput.TraitProfileFacts{Detail: reporttypology.TraitProfileReportDetail{
					Traits: []reporttypology.TraitProfileFactorReport{{Code: "type_1", Name: "完美型", RawScore: 8}},
				}}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			builder := rendering.NewTypologyBuilder()
			input := interpinput.InterpretationInput{
				OutcomeID:   meta.FromUint64(100),
				Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(101), TesteeID: 102},
				Model: report.ModelIdentity{
					Kind: string(modelcatalog.KindTypology), SubKind: string(modelcatalog.SubKindTypology), Algorithm: string(modelcatalog.AlgorithmPersonalityTypology),
					Code: "TYPOLOGY", Version: "v1", Title: "人格测评", AlgorithmFamily: string(modelcatalog.AlgorithmFamilyFactorClassification),
				},
				Runtime: interpinput.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: tc.decisionKind},
				Result:  interpinput.ResultFacts{Primary: report.NewMatchPercentScore(88, "88%"), Level: &report.ResultLevel{Code: "TYPE", Label: "类型"}},
				Report: interpinput.ReportSpec{
					ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
					TemplateID: tc.templateID, AdapterKey: string(tc.adapterKey),
				},
			}
			tc.facts(&input)

			draft, err := builder.Build(context.Background(), input)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			artifact, err := report.NewInterpretReport(report.InterpretReportInput{
				ID: meta.FromUint64(103), GenerationID: meta.FromUint64(104), OutcomeID: input.OutcomeID, InterpretationRunID: meta.FromUint64(105),
				Association: input.Association, ReportType: input.Report.ReportType, TemplateVersion: input.Report.TemplateVersion,
				BuilderIdentity: builder.BuilderIdentity(), ContentSchemaVersion: builder.ContentSchemaVersion(),
				Content: draft.Content(), GeneratedAt: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("NewInterpretReport: %v", err)
			}
			content := artifact.Content()
			if content.Model != input.Model {
				t.Fatalf("model identity = %#v, want %#v", content.Model, input.Model)
			}
			if content.ModelExtra == nil || content.ModelExtra.Kind != tc.wantKind {
				t.Fatalf("model extra = %#v, want kind %q", content.ModelExtra, tc.wantKind)
			}
			if content.ModelExtra.TypeCode == "" && content.ModelExtra.TypeName == "" {
				t.Fatalf("model extra type identity is empty: %#v", content.ModelExtra)
			}
			if len(content.Dimensions) == 0 {
				t.Fatal("typology dimensions are empty")
			}
		})
	}
}
