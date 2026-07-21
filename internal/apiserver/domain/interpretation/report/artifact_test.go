package report

import (
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestInterpretReportIsSuccessOnlyAndDefensivelyCopiesContent(t *testing.T) {
	max := 100.0
	factor := NewFactorCode("sleep")
	input := InterpretReportInput{
		ID:                   meta.FromUint64(1),
		GenerationID:         meta.FromUint64(2),
		OutcomeID:            meta.FromUint64(3),
		InterpretationRunID:  meta.FromUint64(4),
		Association:          Association{OrgID: 7, AssessmentID: meta.FromUint64(5), TesteeID: 6},
		ReportType:           policy.ReportTypeStandard,
		TemplateVersion:      policy.TemplateVersion("v1"),
		BuilderIdentity:      BuilderIdentityFactorScoring,
		ContentSchemaVersion: ContentSchemaVersionV1,
		GeneratedAt:          time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC),
		Content: Content{
			Model: ModelIdentity{Kind: "scale", Code: "PHQ9", Version: "v1"},
			PrimaryScore: &ScoreValue{Kind: ScoreKindRawTotal, Value: 42, Max: &max},
			Dimensions: []DimensionInterpret{
				NewDimensionInterpret(NewFactorCode("sleep"), "sleep", 10, &max, RiskLevelLow, "low", "rest"),
			},
			Suggestions: []Suggestion{{Category: SuggestionCategoryDimension, Content: "rest", FactorCode: &factor}},
		},
	}
	artifact, err := NewInterpretReport(input)
	if err != nil {
		t.Fatal(err)
	}

	input.Content.PrimaryScore.Value = 0
	*input.Content.Dimensions[0].maxScore = 0
	input.Content.Suggestions[0].Content = "changed"
	content := artifact.Content()
	if content.PrimaryScore == nil || content.PrimaryScore.Value != 42 || content.Dimensions[0].MaxScore() == nil || *content.Dimensions[0].MaxScore() != 100 || content.Suggestions[0].Content != "rest" {
		t.Fatalf("artifact retained mutable input: %#v", content)
	}
	content.PrimaryScore.Value = 99
	*content.Dimensions[0].maxScore = 99
	content.Suggestions[0].Content = "caller mutation"
	if got := artifact.Content(); got.PrimaryScore.Value != 42 || got.Dimensions[0].MaxScore() == nil || *got.Dimensions[0].MaxScore() != 100 || got.Suggestions[0].Content != "rest" {
		t.Fatalf("artifact exposed mutable content: %#v", got)
	}

	legacyLifecycleFields := map[string]struct{}{"status": {}, "attempt": {}, "failureReason": {}, "generatingAt": {}, "failedAt": {}}
	typ := reflect.TypeOf(*artifact)
	for i := 0; i < typ.NumField(); i++ {
		if _, found := legacyLifecycleFields[typ.Field(i).Name]; found {
			t.Fatalf("artifact must not contain lifecycle field %q", typ.Field(i).Name)
		}
	}
}

func TestInterpretReportRejectsMissingProvenance(t *testing.T) {
	content := factorScoringMinimalContentForTest()
	_, err := NewInterpretReport(InterpretReportInput{
		ID: meta.FromUint64(1), GenerationID: meta.FromUint64(2), OutcomeID: meta.FromUint64(3),
		InterpretationRunID: meta.FromUint64(4),
		Association:         Association{OrgID: 1, AssessmentID: meta.FromUint64(5), TesteeID: 6},
		ReportType:          policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
		Content: content, GeneratedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("artifact accepted missing provenance")
	}
}

func factorScoringMinimalContentForTest() Content {
	return Content{
		Model:        ModelIdentity{Kind: "scale", Code: "PHQ9", Version: "v1"},
		PrimaryScore: NewRawTotalScore(8, nil),
		Dimensions: []DimensionInterpret{
			NewDimensionInterpret(NewFactorCode("TOTAL"), "总分", 8, nil, RiskLevelLow, "ok", "ok"),
		},
	}
}

func TestInterpretReportRejectsMissingOrganization(t *testing.T) {
	_, err := NewInterpretReport(InterpretReportInput{
		ID:                  meta.FromUint64(1),
		GenerationID:        meta.FromUint64(2),
		OutcomeID:           meta.FromUint64(3),
		InterpretationRunID: meta.FromUint64(4),
		Association:         Association{AssessmentID: meta.FromUint64(5), TesteeID: 6},
		ReportType:          policy.ReportTypeStandard,
		TemplateVersion:     policy.TemplateVersionV1,
		GeneratedAt:         time.Now(),
	})
	if err == nil {
		t.Fatal("artifact accepted missing organization")
	}
}
