package evaluationinput

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScaleToSnapshotMapsFactorScoringAndInterpretRules(t *testing.T) {
	maxScore := 100.0
	factor, err := scale.NewFactor(
		scale.NewFactorCode("total"),
		"总分",
		scale.WithIsTotalScore(true),
		scale.WithQuestionCodes([]meta.Code{meta.NewCode("Q1"), meta.NewCode("Q2")}),
		scale.WithScoringStrategy(scale.ScoringStrategyCnt),
		scale.WithScoringParams(scale.NewScoringParams().WithCntOptionContents([]string{"经常"})),
		scale.WithMaxScore(&maxScore),
		scale.WithInterpretRules([]scale.InterpretationRule{
			scale.NewInterpretationRule(scale.NewScoreRange(0, 60), scale.RiskLevelLow, "低风险", "保持"),
			scale.NewInterpretationRule(scale.NewScoreRange(60, 100), scale.RiskLevelHigh, "高风险", "干预"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor returned error: %v", err)
	}
	medicalScale, err := scale.NewMedicalScale(
		meta.NewCode("SDS"),
		"SDS",
		scale.WithQuestionnaire(meta.NewCode("Q-SDS"), "1.0.0"),
		scale.WithStatus(scale.StatusPublished),
		scale.WithFactors([]*scale.Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale returned error: %v", err)
	}

	snapshot := scaleToSnapshot(medicalScale)
	if snapshot == nil {
		t.Fatal("snapshot is nil")
	}
	if snapshot.Code != "SDS" || snapshot.QuestionnaireCode != "Q-SDS" || snapshot.QuestionnaireVersion != "1.0.0" {
		t.Fatalf("unexpected scale snapshot: %#v", snapshot)
	}
	if len(snapshot.Factors) != 1 {
		t.Fatalf("factor count = %d, want 1", len(snapshot.Factors))
	}
	got := snapshot.Factors[0]
	if got.Code != "total" || got.Title != "总分" || !got.IsTotalScore {
		t.Fatalf("unexpected factor snapshot: %#v", got)
	}
	if got.ScoringStrategy != "cnt" || len(got.ScoringParams.CntOptionContents) != 1 || got.ScoringParams.CntOptionContents[0] != "经常" {
		t.Fatalf("unexpected scoring params: %#v", got.ScoringParams)
	}
	if got.MaxScore == nil || *got.MaxScore != maxScore {
		t.Fatalf("max score = %v, want %v", got.MaxScore, maxScore)
	}
	if len(got.InterpretRules) != 2 || got.InterpretRules[1].RiskLevel != "high" || got.InterpretRules[1].Conclusion != "高风险" {
		t.Fatalf("unexpected interpret rules: %#v", got.InterpretRules)
	}
}
