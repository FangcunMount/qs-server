package evaluation

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
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

func TestScorePOsToReadRowUsesSingleNonTotalFactorForTrendRows(t *testing.T) {
	scaleID := uint64(3001)
	row := scorePOsToReadRow([]*AssessmentScorePO{
		{
			AssessmentID:     102,
			MedicalScaleID:   scaleID,
			MedicalScaleCode: "SDS",
			FactorCode:       "sleep",
			FactorName:       "睡眠",
			RawScore:         12,
			RiskLevel:        "medium",
		},
	})

	if row.AssessmentID != 102 || row.TotalScore != 12 || row.RiskLevel != "medium" {
		t.Fatalf("unexpected trend row summary: %#v", row)
	}
	if len(row.FactorScores) != 1 || row.FactorScores[0].FactorCode != "sleep" || row.FactorScores[0].IsTotalScore {
		t.Fatalf("unexpected trend factor rows: %#v", row.FactorScores)
	}
}

func TestApplyAssessmentReadModelFilterBuildsExpectedWhereClauses(t *testing.T) {
	conn, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/qs_server_dry_run?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("open dry-run sql db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{
		Conn:                      conn,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run gorm db: %v", err)
	}
	testeeID := uint64(2001)
	from := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	query := applyAssessmentReadModelFilter(
		db.Session(&gorm.Session{DryRun: true}).Model(&AssessmentPO{}).Where("deleted_at IS NULL"),
		evaluationreadmodel.AssessmentFilter{
			OrgID:                 9,
			TesteeID:              &testeeID,
			RestrictToAccessScope: true,
			AccessibleTesteeIDs:   []uint64{2001, 2002},
			Statuses:              []string{"submitted"},
			ScaleCode:             "SDS",
			RiskLevel:             "HIGH",
			DateFrom:              &from,
			DateTo:                &to,
		},
	)

	var rows []AssessmentPO
	stmt := query.Find(&rows).Statement
	sql := stmt.SQL.String()
	for _, token := range []string{
		"deleted_at IS NULL",
		"org_id = ?",
		"testee_id = ?",
		"testee_id IN",
		"status IN",
		"medical_scale_code = ?",
		"risk_level = ?",
		"created_at >= ?",
		"created_at < ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	if !containsVar(stmt.Vars, "high") {
		t.Fatalf("query vars = %#v, want lower-case risk level", stmt.Vars)
	}
}

func TestLatestRiskQueueQuerySupportsRestrictedAndAllOrgScopes(t *testing.T) {
	restrictedSQL := latestRiskQueueRowsQuery(true)
	for _, token := range []string{
		"ROW_NUMBER() OVER",
		"PARTITION BY assessment.testee_id",
		"assessment.org_id = ?",
		"assessment.testee_id IN ?",
		"assessment.status = ?",
		"LOWER(ranked.risk_level) IN ?",
		"ORDER BY occurred_at DESC, assessment_id DESC",
		"LIMIT ? OFFSET ?",
	} {
		if !strings.Contains(restrictedSQL, token) {
			t.Fatalf("restricted latest risk query does not contain %q:\n%s", token, restrictedSQL)
		}
	}

	allOrgSQL := latestRiskQueueRowsQuery(false)
	if strings.Contains(allOrgSQL, "assessment.testee_id IN ?") {
		t.Fatalf("all-org latest risk query should not restrict testee ids:\n%s", allOrgSQL)
	}
	if !strings.Contains(allOrgSQL, "assessment.org_id = ?") || !strings.Contains(allOrgSQL, "LOWER(ranked.risk_level) IN ?") {
		t.Fatalf("all-org latest risk query lost org/risk filters:\n%s", allOrgSQL)
	}
}

func TestBuildFactorTrendQueryDocumentsFilterOrderAndLimit(t *testing.T) {
	conn, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/qs_server_dry_run?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("open dry-run sql db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{
		Conn:                      conn,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run gorm db: %v", err)
	}

	var rows []AssessmentScorePO
	stmt := buildFactorTrendQuery(
		db.Session(&gorm.Session{DryRun: true}).Model(&AssessmentScorePO{}),
		evaluationreadmodel.FactorTrendFilter{TesteeID: 2001, FactorCode: "sleep", Limit: 5},
	).Find(&rows).Statement
	sql := stmt.SQL.String()
	for _, token := range []string{
		"testee_id = ?",
		"factor_code = ?",
		"deleted_at IS NULL",
		"ORDER BY id DESC",
		"LIMIT ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{uint64(2001), "sleep", 5} {
		if !containsVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
	}
}

func TestLatestRiskRowsQueryDocumentsCurrentRiskPerTesteeContract(t *testing.T) {
	for _, token := range []string{
		"ROW_NUMBER() OVER",
		"PARTITION BY assessment.testee_id",
		"ORDER BY COALESCE(assessment.interpreted_at, assessment.updated_at, assessment.created_at) DESC, assessment.id DESC",
		"assessment.org_id = ?",
		"assessment.testee_id IN ?",
		"assessment.status = ?",
		"assessment.risk_level IS NOT NULL",
		"assessment.risk_level <> ''",
		"assessment.deleted_at IS NULL",
		"ranked.row_num = 1",
	} {
		if !strings.Contains(latestRiskRowsQuery, token) {
			t.Fatalf("latest risk query does not contain %q:\n%s", token, latestRiskRowsQuery)
		}
	}
}

func containsVar(values []interface{}, want interface{}) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
