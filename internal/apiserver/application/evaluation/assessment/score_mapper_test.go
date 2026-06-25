package assessment

import (
	"testing"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func TestScoreRowToResultUsesRulesetFactorMaxScores(t *testing.T) {
	maxScore := 27.0
	row := &evaluationreadmodel.ScoreRow{
		AssessmentID: 1001,
		TotalScore:   18,
		RiskLevel:    string(domainAssessment.RiskLevelMedium),
		FactorScores: []evaluationreadmodel.ScoreFactorRow{
			{
				FactorCode:   "total",
				FactorName:   "总分",
				RawScore:     18,
				RiskLevel:    string(domainAssessment.RiskLevelMedium),
				IsTotalScore: true,
			},
		},
	}
	scale := &scalesnapshot.ScaleSnapshot{
		Factors: []scalesnapshot.FactorSnapshot{
			{Code: "total", MaxScore: &maxScore, IsTotalScore: true},
		},
	}

	got := scoreRowToResult(row, scale)
	if got == nil {
		t.Fatal("expected score result")
	}
	if len(got.FactorScores) != 1 {
		t.Fatalf("factor count = %d, want 1", len(got.FactorScores))
	}
	if got.FactorScores[0].MaxScore == nil || *got.FactorScores[0].MaxScore != maxScore {
		t.Fatalf("max score = %#v, want %.1f", got.FactorScores[0].MaxScore, maxScore)
	}
}

func TestHighRiskFactorsResultFromScoreRowFlagsSevereOverallRisk(t *testing.T) {
	row := &evaluationreadmodel.ScoreRow{
		AssessmentID: 2002,
		TotalScore:   40,
		RiskLevel:    string(domainAssessment.RiskLevelSevere),
		FactorScores: []evaluationreadmodel.ScoreFactorRow{
			{FactorCode: "total", RawScore: 40, RiskLevel: string(domainAssessment.RiskLevelLow)},
		},
	}

	got := highRiskFactorsResultFromScoreRow(2002, row, nil)
	if got == nil {
		t.Fatal("expected high risk result")
	}
	if !got.NeedsUrgentCare {
		t.Fatal("expected urgent care for severe overall risk")
	}
}

func TestFactorMaxScoresReturnsEmptyMapForNilScale(t *testing.T) {
	got := factorMaxScores(nil)
	if len(got) != 0 {
		t.Fatalf("factor max scores = %#v, want empty map", got)
	}
}
