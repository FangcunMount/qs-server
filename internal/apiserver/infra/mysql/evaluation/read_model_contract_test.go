package evaluation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAssessmentPOToReadRowMapsAllReadModelFields(t *testing.T) {
	scaleID := uint64(3001)
	scaleCode := "SDS"
	scaleName := "抑郁自评"
	originID := "plan-1"
	total := 88.5
	risk := "high"
	now := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)
	failure := "engine failed"

	row := assessmentPOToReadRow(&AssessmentPO{
		AuditFields:          mysql.AuditFields{ID: meta.FromUint64(101)},
		OrgID:                1,
		TesteeID:             2001,
		QuestionnaireCode:    "Q-SDS",
		QuestionnaireVersion: "1.0.0",
		MedicalScaleID:       &scaleID,
		MedicalScaleCode:     &scaleCode,
		MedicalScaleName:     &scaleName,
		AnswerSheetID:        5001,
		OriginType:           "plan",
		OriginID:             &originID,
		Status:               "interpreted",
		TotalScore:           &total,
		RiskLevel:            &risk,
		SubmittedAt:          &now,
		InterpretedAt:        &now,
		FailedAt:             &now,
		FailureReason:        &failure,
	})

	if row.ID != 101 || row.OrgID != 1 || row.TesteeID != 2001 || row.AnswerSheetID != 5001 {
		t.Fatalf("unexpected identity fields: %#v", row)
	}
	if row.QuestionnaireCode != "Q-SDS" || row.QuestionnaireVersion != "1.0.0" {
		t.Fatalf("unexpected questionnaire fields: %#v", row)
	}
	if row.MedicalScaleID == nil || *row.MedicalScaleID != scaleID || row.MedicalScaleCode == nil || *row.MedicalScaleCode != scaleCode {
		t.Fatalf("unexpected scale fields: %#v", row)
	}
	if row.OriginID == nil || *row.OriginID != originID || row.TotalScore == nil || *row.TotalScore != total || row.RiskLevel == nil || *row.RiskLevel != risk {
		t.Fatalf("unexpected optional fields: %#v", row)
	}
	if row.SubmittedAt == nil || !row.SubmittedAt.Equal(now) || row.FailureReason == nil || *row.FailureReason != failure {
		t.Fatalf("unexpected time/failure fields: %#v", row)
	}
}

func TestScorePOsToReadRowUsesTotalScoreFactorForSummaryAndOrdersRowsAsProvided(t *testing.T) {
	scaleID := uint64(3001)
	rows := scorePOsToReadRow([]*AssessmentScorePO{
		{
			AssessmentID:     101,
			MedicalScaleID:   scaleID,
			MedicalScaleCode: "SDS",
			FactorCode:       "total",
			FactorName:       "总分",
			IsTotalScore:     true,
			RawScore:         88,
			RiskLevel:        "high",
			Conclusion:       "high risk",
			Suggestion:       "follow",
		},
		{
			AssessmentID:     101,
			MedicalScaleID:   scaleID,
			MedicalScaleCode: "SDS",
			FactorCode:       "sleep",
			FactorName:       "睡眠",
			RawScore:         12,
			RiskLevel:        "medium",
		},
	})

	if rows.AssessmentID != 101 || rows.TotalScore != 88 || rows.RiskLevel != "high" {
		t.Fatalf("unexpected summary row: %#v", rows)
	}
	if rows.MedicalScaleID == nil || *rows.MedicalScaleID != scaleID || rows.MedicalScaleCode == nil || *rows.MedicalScaleCode != "SDS" {
		t.Fatalf("unexpected scale metadata: %#v", rows)
	}
	if len(rows.FactorScores) != 2 || rows.FactorScores[0].FactorCode != "total" || rows.FactorScores[1].FactorCode != "sleep" {
		t.Fatalf("unexpected factor rows: %#v", rows.FactorScores)
	}
	if !rows.FactorScores[0].IsTotalScore || rows.FactorScores[0].Conclusion != "high risk" || rows.FactorScores[0].Suggestion != "follow" {
		t.Fatalf("unexpected total factor row: %#v", rows.FactorScores[0])
	}
}
