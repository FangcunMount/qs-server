package mbti

import (
	"testing"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestReportBuilderFillsModelExtra(t *testing.T) {
	detail := evaluationdomain.MBTIResultDetail{
		TypeCode:     "INTJ",
		TypeName:     "建筑师",
		OneLiner:     "独立战略家",
		MatchPercent: 75,
		Profile: rulesetmbti.TypeProfileSnapshot{
			TypeCode: "INTJ",
			TypeName: "建筑师",
			Summary:  "善于长远规划",
		},
		Source: rulesetmbti.SourceSnapshot{
			Attribution:   "OEJTS",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
	}
	a, err := assessment.NewAssessment(
		1, 1,
		assessment.NewQuestionnaireRefByCode(meta.NewCode("MBTI_OEJTS"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7001)),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	result := assessment.NewModelEvaluationResult(
		assessment.NewEvaluationModelRefByCode(assessment.EvaluationModelKindMBTI, meta.NewCode("MBTI_OEJTS"), "1.0.0", "MBTI"),
		assessment.ResultSummary{PrimaryLabel: "INTJ"},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindMBTI, Payload: detail},
	)

	report, err := NewReportBuilder().Build(t.Context(), evaluationresult.Outcome{
		Assessment: a,
		Result:     result,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	extra := report.ModelExtra()
	if extra == nil {
		t.Fatal("expected model extra")
	}
	if extra.Kind != "mbti" || extra.TypeCode != "INTJ" || extra.TypeName != "建筑师" {
		t.Fatalf("unexpected model extra: %#v", extra)
	}
	if extra.MatchPercent != 75 {
		t.Fatalf("MatchPercent = %v, want 75", extra.MatchPercent)
	}
}
