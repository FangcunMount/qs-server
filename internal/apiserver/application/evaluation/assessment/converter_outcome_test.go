package assessment

import (
	"testing"

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

func TestLegacyRiskLevelResultPreservesScoreFactProjection(t *testing.T) {
	tests := []struct {
		code     string
		severity string
		wantNil  bool
	}{
		{code: "severe", severity: "high"},
		{code: "high", severity: "high"},
		{code: "medium", severity: "medium"},
		{code: "low", severity: "low"},
		{code: "none", severity: "none"},
		{code: "INTJ", wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := legacyRiskLevelResult(tt.code)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("legacyRiskLevelResult(%q) = %#v, want nil", tt.code, got)
				}
				return
			}
			if got == nil || got.Code != tt.code || got.Label != tt.code || got.Severity != tt.severity {
				t.Fatalf("legacyRiskLevelResult(%q) = %#v", tt.code, got)
			}
		})
	}
}

func strPtr(v string) *string {
	return &v
}
