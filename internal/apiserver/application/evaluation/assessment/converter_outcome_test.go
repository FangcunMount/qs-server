package assessment

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func TestAssessmentRowToOutcomeResultUsesOutcomeColumns(t *testing.T) {
	kind := "scale"
	algorithm := "scale_default"
	scoreKind := "raw_total"
	scoreValue := 21.0
	levelCode := "medium"
	levelLabel := "中等"
	severity := "medium"
	row := evaluationreadmodel.AssessmentRow{
		ID:                       101,
		OrgID:                    12,
		TesteeID:                 34,
		QuestionnaireCode:        "Q-SDS",
		QuestionnaireVersion:     "1.0.0",
		AnswerSheetID:            501,
		EvaluationModelKind:      &kind,
		EvaluationModelAlgorithm: &algorithm,
		EvaluationModelCode:      strPtr("SDS"),
		PrimaryScoreKind:         &scoreKind,
		PrimaryScoreValue:        &scoreValue,
		LevelCode:                &levelCode,
		LevelLabel:               &levelLabel,
		Severity:                 &severity,
		OriginType:               "adhoc",
		Status:                   "interpreted",
	}

	result, err := assessmentRowToOutcomeResult(row)
	if err != nil {
		t.Fatalf("assessmentRowToOutcomeResult returned error: %v", err)
	}
	if result.Model.Kind != "scale" || result.Model.Algorithm != "scale_default" || result.Model.Code != "SDS" {
		t.Fatalf("unexpected model identity: %#v", result.Model)
	}
	if result.PrimaryScore == nil || result.PrimaryScore.Kind != scoreKind || result.PrimaryScore.Value != scoreValue {
		t.Fatalf("unexpected primary score: %#v", result.PrimaryScore)
	}
	if result.Level == nil || result.Level.Code != levelCode || result.Level.Severity != severity {
		t.Fatalf("unexpected level: %#v", result.Level)
	}
}

func TestReportRowToOutcomeResultPrefersExplicitModelProjection(t *testing.T) {
	row := evaluationreadmodel.ReportRow{
		AssessmentID: 202,
		Model: evaluationreadmodel.ModelIdentityRow{
			Kind:            "personality",
			SubKind:         "typology",
			Algorithm:       "mbti",
			Code:            "MBTI-16P",
			Title:           "MBTI",
			ProductChannel:  "behavior_ability",
			AlgorithmFamily: "typology",
		},
		PrimaryScore: &evaluationreadmodel.ScoreValueRow{
			Kind:  "match_percent",
			Value: 88,
			Label: "88%",
		},
		Level: &evaluationreadmodel.ResultLevelRow{
			Code:     "INTJ",
			Label:    "INTJ",
			Severity: "none",
		},
		Conclusion: "建筑师",
		CreatedAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	result := reportRowToOutcomeResult(row)
	if result.Model.Kind != "personality" || result.Model.SubKind != "typology" {
		t.Fatalf("unexpected model: %#v", result.Model)
	}
	if result.Model.ProductChannel != "behavior_ability" {
		t.Fatalf("product_channel = %q, want behavior_ability", result.Model.ProductChannel)
	}
	if result.Model.AlgorithmFamily == "" {
		t.Fatal("expected derived algorithm_family")
	}
	if result.PrimaryScore == nil || result.PrimaryScore.Kind != "match_percent" {
		t.Fatalf("unexpected primary score: %#v", result.PrimaryScore)
	}
	if result.Level == nil || result.Level.Code != "INTJ" {
		t.Fatalf("unexpected level: %#v", result.Level)
	}
}

func strPtr(v string) *string {
	return &v
}
