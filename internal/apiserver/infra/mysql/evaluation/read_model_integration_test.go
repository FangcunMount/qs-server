package evaluation

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func openEvaluationMySQLContractDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set QS_SERVER_TEST_MYSQL_DSN to run MySQL evaluation read model contract tests")
	}

	db, err := gorm.Open(mysqlDriver.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open mysql test db: %v", err)
	}
	if err := db.AutoMigrate(&AssessmentPO{}, &AssessmentScorePO{}); err != nil {
		t.Fatalf("auto migrate evaluation tables: %v", err)
	}
	return db
}

func TestAssessmentReadModelListAssessmentsFiltersAgainstDatabase(t *testing.T) {
	db := openEvaluationMySQLContractDB(t)
	ctx := context.Background()

	base := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	orgID := int64(base%100000 + 1000)
	testeeID := base + 1000
	scaleCode := fmt.Sprintf("scale-%d", base)
	scaleName := "抑郁自评"
	riskHigh := "high"
	riskMedium := "medium"
	totalScore := 88.0
	now := time.Now().UTC().Truncate(time.Second)

	rows := []AssessmentPO{
		{
			AuditFields:          mysql.AuditFields{ID: meta.FromUint64(base + 1), CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now},
			OrgID:                orgID,
			TesteeID:             testeeID,
			QuestionnaireCode:    "Q-SDS",
			QuestionnaireVersion: "1.0.0",
			MedicalScaleCode:     &scaleCode,
			MedicalScaleName:     &scaleName,
			AnswerSheetID:        base + 101,
			OriginType:           "adhoc",
			Status:               "interpreted",
			TotalScore:           &totalScore,
			RiskLevel:            &riskHigh,
			InterpretedAt:        ptrTime(now.Add(-20 * time.Minute)),
		},
		{
			AuditFields:          mysql.AuditFields{ID: meta.FromUint64(base + 2), CreatedAt: now.Add(-25 * time.Minute), UpdatedAt: now},
			OrgID:                orgID,
			TesteeID:             testeeID,
			QuestionnaireCode:    "Q-SDS",
			QuestionnaireVersion: "1.0.0",
			MedicalScaleCode:     &scaleCode,
			MedicalScaleName:     &scaleName,
			AnswerSheetID:        base + 102,
			OriginType:           "adhoc",
			Status:               "interpreted",
			TotalScore:           &totalScore,
			RiskLevel:            &riskMedium,
		},
		{
			AuditFields:          mysql.AuditFields{ID: meta.FromUint64(base + 3), CreatedAt: now.Add(-20 * time.Minute), UpdatedAt: now},
			OrgID:                orgID,
			TesteeID:             testeeID + 1,
			QuestionnaireCode:    "Q-SDS",
			QuestionnaireVersion: "1.0.0",
			MedicalScaleCode:     &scaleCode,
			MedicalScaleName:     &scaleName,
			AnswerSheetID:        base + 103,
			OriginType:           "adhoc",
			Status:               "interpreted",
			TotalScore:           &totalScore,
			RiskLevel:            &riskHigh,
		},
		{
			AuditFields:          mysql.AuditFields{ID: meta.FromUint64(base + 4), CreatedAt: now.Add(-15 * time.Minute), UpdatedAt: now},
			OrgID:                orgID,
			TesteeID:             testeeID,
			QuestionnaireCode:    "Q-SDS",
			QuestionnaireVersion: "1.0.0",
			MedicalScaleCode:     &scaleCode,
			MedicalScaleName:     &scaleName,
			AnswerSheetID:        base + 104,
			OriginType:           "adhoc",
			Status:               "submitted",
			TotalScore:           &totalScore,
			RiskLevel:            &riskHigh,
		},
	}
	if err := db.WithContext(ctx).Create(&rows).Error; err != nil {
		t.Fatalf("insert assessments: %v", err)
	}
	t.Cleanup(func() {
		ids := []uint64{base + 1, base + 2, base + 3, base + 4}
		_ = db.Exec("DELETE FROM assessment WHERE id IN ?", ids).Error
	})

	from := now.Add(-45 * time.Minute)
	to := now.Add(-10 * time.Minute)
	reader := NewAssessmentReadModel(db)
	got, total, err := reader.ListAssessments(ctx, evaluationreadmodel.AssessmentFilter{
		OrgID:                 orgID,
		TesteeID:              &testeeID,
		RestrictToAccessScope: true,
		AccessibleTesteeIDs:   []uint64{testeeID},
		Statuses:              []string{"interpreted"},
		ScaleCode:             scaleCode,
		RiskLevel:             "HIGH",
		DateFrom:              &from,
		DateTo:                &to,
	}, evaluationreadmodel.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list assessments: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ID != base+1 {
		t.Fatalf("filtered rows = %#v total=%d, want only id %d", got, total, base+1)
	}
	if got[0].RiskLevel == nil || *got[0].RiskLevel != "high" || got[0].MedicalScaleCode == nil || *got[0].MedicalScaleCode != scaleCode {
		t.Fatalf("unexpected mapped row: %#v", got[0])
	}

	got, total, err = reader.ListAssessments(ctx, evaluationreadmodel.AssessmentFilter{
		OrgID:                 orgID,
		RestrictToAccessScope: true,
	}, evaluationreadmodel.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list empty access scope: %v", err)
	}
	if total != 0 || len(got) != 0 {
		t.Fatalf("empty access scope rows = %#v total=%d, want none", got, total)
	}
}

func TestScoreReadModelFiltersAndOrdersAgainstDatabase(t *testing.T) {
	db := openEvaluationMySQLContractDB(t)
	ctx := context.Background()

	base := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	testeeID := base + 2000
	scaleID := base + 3000
	scaleCode := fmt.Sprintf("scale-score-%d", base)
	assessmentID := base + 10

	scores := []AssessmentScorePO{
		{
			AuditFields:      mysql.AuditFields{ID: meta.FromUint64(base + 20)},
			AssessmentID:     assessmentID,
			TesteeID:         testeeID,
			MedicalScaleID:   scaleID,
			MedicalScaleCode: scaleCode,
			FactorCode:       "sleep",
			FactorName:       "睡眠",
			RawScore:         12,
			RiskLevel:        "medium",
		},
		{
			AuditFields:      mysql.AuditFields{ID: meta.FromUint64(base + 21)},
			AssessmentID:     assessmentID,
			TesteeID:         testeeID,
			MedicalScaleID:   scaleID,
			MedicalScaleCode: scaleCode,
			FactorCode:       "total",
			FactorName:       "总分",
			IsTotalScore:     true,
			RawScore:         88,
			RiskLevel:        "high",
			Conclusion:       "高风险",
			Suggestion:       "建议干预",
		},
		{
			AuditFields:      mysql.AuditFields{ID: meta.FromUint64(base + 22)},
			AssessmentID:     assessmentID,
			TesteeID:         testeeID,
			MedicalScaleID:   scaleID,
			MedicalScaleCode: scaleCode,
			FactorCode:       "anxiety",
			FactorName:       "焦虑",
			RawScore:         8,
			RiskLevel:        "low",
		},
		{
			AuditFields:      mysql.AuditFields{ID: meta.FromUint64(base + 23)},
			AssessmentID:     assessmentID + 1,
			TesteeID:         testeeID,
			MedicalScaleID:   scaleID,
			MedicalScaleCode: scaleCode,
			FactorCode:       "sleep",
			FactorName:       "睡眠",
			RawScore:         16,
			RiskLevel:        "high",
		},
	}
	if err := db.WithContext(ctx).Create(&scores).Error; err != nil {
		t.Fatalf("insert scores: %v", err)
	}
	t.Cleanup(func() {
		ids := []uint64{base + 20, base + 21, base + 22, base + 23}
		_ = db.Exec("DELETE FROM assessment_score WHERE id IN ?", ids).Error
	})

	reader := NewScoreReadModel(db)
	score, err := reader.GetScoreByAssessmentID(ctx, assessmentID)
	if err != nil {
		t.Fatalf("get score: %v", err)
	}
	if score.TotalScore != 88 || score.RiskLevel != "high" {
		t.Fatalf("unexpected score summary: %#v", score)
	}
	if len(score.FactorScores) != 3 {
		t.Fatalf("factor count = %d, want 3: %#v", len(score.FactorScores), score.FactorScores)
	}
	if score.FactorScores[0].FactorCode != "total" || score.FactorScores[1].FactorCode != "anxiety" || score.FactorScores[2].FactorCode != "sleep" {
		t.Fatalf("factor order = %#v, want total then factor_code asc", score.FactorScores)
	}

	trend, err := reader.ListFactorTrend(ctx, evaluationreadmodel.FactorTrendFilter{
		TesteeID:   testeeID,
		FactorCode: "sleep",
		Limit:      1,
	})
	if err != nil {
		t.Fatalf("list factor trend: %v", err)
	}
	if len(trend) != 1 || trend[0].AssessmentID != assessmentID+1 || trend[0].TotalScore != 16 {
		t.Fatalf("trend = %#v, want latest sleep score only", trend)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
