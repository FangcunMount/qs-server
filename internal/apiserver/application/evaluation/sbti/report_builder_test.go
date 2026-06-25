package sbti

import (
	"testing"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/sbti"
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestReportBuilderSetsModelExtra(t *testing.T) {
	detail := evaluationsbti.ResultDetail{
		TypeCode:   "CTRL",
		TypeName:   "拿捏者",
		OneLiner:   "人形自走任务管理器",
		Similarity: 0.92,
		ImageURL:   "https://example.com/CTRL.png",
		Rarity: rulesetsbti.RaritySnapshot{
			Percent: 3.61,
			Label:   "中等",
			OneInX:  28,
		},
		Outcome: rulesetsbti.OutcomeSnapshot{
			Code:       "CTRL",
			Name:       "拿捏者",
			Commentary: "测试解读",
		},
	}
	a, err := assessment.NewAssessment(
		1, 1,
		assessment.NewQuestionnaireRefByCode(meta.NewCode(port.DefaultSBTIQuestionnaireCode), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7001)),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	outcome := evaluationresult.Outcome{
		Assessment: a,
		Result: assessment.NewModelEvaluationResult(
			assessment.NewEvaluationModelRefByCode(assessment.EvaluationModelKindSBTI, meta.NewCode(port.DefaultSBTIModelCode), port.DefaultSBTIModelVersion, port.DefaultSBTIModelTitle),
			assessment.ResultSummary{PrimaryLabel: "CTRL", Score: ptrFloat(92)},
			assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindSBTI, Payload: detail},
		),
	}

	report, err := NewReportBuilder().Build(t.Context(), outcome)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	extra := report.ModelExtra()
	if extra == nil {
		t.Fatal("expected model extra")
	}
	if extra.TypeCode != "CTRL" {
		t.Fatalf("TypeCode = %s, want CTRL", extra.TypeCode)
	}
	if extra.ImageURL == "" {
		t.Fatal("expected image url")
	}
	if extra.Rarity == nil || extra.Rarity.OneInX != 28 {
		t.Fatalf("rarity = %#v, want one_in_x 28", extra.Rarity)
	}
	if extra.MatchPercent != 92 {
		t.Fatalf("MatchPercent = %.2f, want 92", extra.MatchPercent)
	}
	if extra.Kind != "sbti" {
		t.Fatalf("Kind = %s, want sbti", extra.Kind)
	}
}

func ptrFloat(v float64) *float64 { return &v }
