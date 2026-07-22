package rendering_test

import (
	"context"
	"errors"
	"testing"
	"time"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
