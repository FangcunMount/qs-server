package characterization_test

import (
	"context"
	"testing"

	typologyapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	mongoevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
)

// V1 contract: MBTI scorer resolves INTJ; report exposes type code, match percent,
// dimension preference text, and profile-derived suggestions.
func TestV1MBTIPipelinePreservesTypeCodeAndReportFields(t *testing.T) {
	model := mbtiINTJModel()
	detail, err := evaluationtypology.ScoreMBTIReference(model, mbtiINTJAnswerSheet())
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if detail.TypeCode != "INTJ" {
		t.Fatalf("TypeCode = %s, want INTJ", detail.TypeCode)
	}
	if detail.TypeName != "建筑师" {
		t.Fatalf("TypeName = %s, want 建筑师", detail.TypeName)
	}
	if detail.MatchPercent != 40 {
		t.Fatalf("MatchPercent = %.2f, want 40", detail.MatchPercent)
	}
	if len(detail.Dimensions) != 4 {
		t.Fatalf("len(Dimensions) = %d, want 4", len(detail.Dimensions))
	}

	a := submittedMBTIAssessment(t)
	modelRef := *a.EvaluationModelRef()
	result := assessment.NewModelEvaluationResult(
		modelRef,
		assessment.ResultSummary{PrimaryLabel: detail.TypeCode, Tags: []string{detail.TypeName}},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindPersonality, Payload: detail},
	)

	report, err := typologyapp.NewMBTIReportBuilder().Build(context.Background(), evaluationresult.NewOutcomeFromLegacyResult(a, nil, result))
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}

	if report.RiskLevel() != domainreport.RiskLevelNone {
		t.Fatalf("RiskLevel = %s, want none", report.RiskLevel())
	}
	if report.Conclusion() != "INTJ 建筑师 - 独立战略家" {
		t.Fatalf("Conclusion = %q, want personality title", report.Conclusion())
	}

	extra := report.ModelExtra()
	if extra == nil || extra.Kind != "mbti" || extra.TypeCode != "INTJ" || extra.TypeName != "建筑师" {
		t.Fatalf("ModelExtra = %#v, want mbti INTJ", extra)
	}
	if extra.MatchPercent != 40 {
		t.Fatalf("MatchPercent = %.2f, want 40", extra.MatchPercent)
	}

	dims := report.Dimensions()
	if len(dims) != 4 {
		t.Fatalf("len(Dimensions) = %d, want 4", len(dims))
	}
	assertDimensionField(t, dims[0], "外向-内向", 23, domainreport.RiskLevelNone, "外向-内向：倾向 I（原始分 23，偏好强度 20%）")

	suggestions := report.Suggestions()
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "善于长远规划")
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "优势：系统思考")
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "注意：表达克制")
	assertSuggestionExists(t, suggestions, domainreport.SuggestionCategoryGeneral, "建议：保留沟通空间")

	mapper := mongoevaluation.NewReportMapper()
	roundTrip := mapper.ToDomain(mapper.ToPO(report, 8002))
	if roundTrip.ModelExtra() == nil || roundTrip.ModelExtra().TypeCode != "INTJ" {
		t.Fatalf("mongo round trip model extra = %#v", roundTrip.ModelExtra())
	}
	if len(roundTrip.Dimensions()) != 4 {
		t.Fatalf("mongo round trip dimensions = %d, want 4", len(roundTrip.Dimensions()))
	}
}
