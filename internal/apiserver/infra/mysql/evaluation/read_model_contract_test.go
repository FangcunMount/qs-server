package evaluation

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAssessmentPOToReadRowMapsAllReadModelFields(t *testing.T) {
	scaleCode := "SDS"
	scaleName := "抑郁自评"
	modelKind := "scale"
	modelVersion := "1.0.0"
	originID := "plan-1"
	total := 88.5
	risk := "high"
	now := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)
	failure := "engine failed"

	row := assessmentPOToReadRow(&AssessmentPO{
		AuditFields:            mysql.AuditFields{ID: meta.FromUint64(101)},
		OrgID:                  1,
		TesteeID:               2001,
		QuestionnaireCode:      "Q-SDS",
		QuestionnaireVersion:   "1.0.0",
		EvaluationModelKind:    &modelKind,
		EvaluationModelCode:    &scaleCode,
		EvaluationModelVersion: &modelVersion,
		EvaluationModelTitle:   &scaleName,
		AnswerSheetID:          5001,
		OriginType:             "plan",
		OriginID:               &originID,
		Status:                 "evaluated",
		TotalScore:             &total,
		RiskLevel:              &risk,
		SubmittedAt:            &now,
		EvaluatedAt:            &now,
		FailedAt:               &now,
		FailureReason:          &failure,
	})

	if row.ID != 101 || row.OrgID != 1 || row.TesteeID != 2001 || row.AnswerSheetID != 5001 {
		t.Fatalf("unexpected identity fields: %#v", row)
	}
	if row.QuestionnaireCode != "Q-SDS" || row.QuestionnaireVersion != "1.0.0" {
		t.Fatalf("unexpected questionnaire fields: %#v", row)
	}
	if row.EvaluationModelKind == nil || *row.EvaluationModelKind != modelKind ||
		row.EvaluationModelCode == nil || *row.EvaluationModelCode != scaleCode ||
		row.EvaluationModelVersion == nil || *row.EvaluationModelVersion != modelVersion ||
		row.EvaluationModelTitle == nil || *row.EvaluationModelTitle != scaleName {
		t.Fatalf("unexpected evaluation model fields: %#v", row)
	}
	if row.OriginID == nil || *row.OriginID != originID || row.TotalScore == nil || *row.TotalScore != total || row.RiskLevel == nil || *row.RiskLevel != risk {
		t.Fatalf("unexpected optional fields: %#v", row)
	}
	if row.SubmittedAt == nil || !row.SubmittedAt.Equal(now) || row.FailureReason == nil || *row.FailureReason != failure {
		t.Fatalf("unexpected time/failure fields: %#v", row)
	}
}

func TestScorePOsToReadRowUsesTotalScoreFactorForSummaryAndOrdersRowsAsProvided(t *testing.T) {
	rows := scorePOsToReadRow([]*AssessmentScorePO{
		{
			AssessmentID: 101,
			FactorCode:   "total",
			FactorName:   "总分",
			IsTotalScore: true,
			RawScore:     88,
			RiskLevel:    "high",
		},
		{
			AssessmentID: 101,
			FactorCode:   "sleep",
			FactorName:   "睡眠",
			RawScore:     12,
			RiskLevel:    "medium",
		},
	})

	if rows.AssessmentID != 101 || rows.TotalScore != 88 || rows.RiskLevel != "high" {
		t.Fatalf("unexpected summary row: %#v", rows)
	}
	if len(rows.FactorScores) != 2 || rows.FactorScores[0].FactorCode != "total" || rows.FactorScores[1].FactorCode != "sleep" {
		t.Fatalf("unexpected factor rows: %#v", rows.FactorScores)
	}
	if !rows.FactorScores[0].IsTotalScore {
		t.Fatalf("unexpected total factor row: %#v", rows.FactorScores[0])
	}
}

func TestScorePOsToReadRowUsesSingleNonTotalFactorForTrendRows(t *testing.T) {
	row := scorePOsToReadRow([]*AssessmentScorePO{
		{
			AssessmentID: 102,
			FactorCode:   "sleep",
			FactorName:   "睡眠",
			RawScore:     12,
			RiskLevel:    "medium",
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
			ModelKinds:            []string{"behavioral_rating", "cognitive"},
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
		"evaluation_model_kind = ? AND evaluation_model_code = ?",
		"evaluation_model_kind IN",
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
		"MAX(assessment.id) AS latest_id",
		"FORCE INDEX (idx_assessment_workbench_latest_id_risk_by_testee)",
		"assessment.org_id = ?",
		"assessment.testee_id IN ?",
		"assessment.status = ?",
		"assessment.deleted_at IS NULL",
		"assessment.risk_level IS NOT NULL",
		"assessment.risk_level <> ''",
		"GROUP BY assessment.testee_id",
		"JOIN assessment a ON a.id = latest.latest_id",
		"a.org_id = ?",
		"a.status = ?",
		"a.risk_level IN ?",
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
	if !strings.Contains(allOrgSQL, "a.org_id = ?") || !strings.Contains(allOrgSQL, "a.risk_level IN ?") {
		t.Fatalf("all-org latest risk query lost org/risk filters:\n%s", allOrgSQL)
	}
	countSQL := latestRiskQueueCountQuery(false)
	if !strings.HasPrefix(strings.TrimSpace(countSQL), "SELECT COUNT(*)") {
		t.Fatalf("latest risk count query should count latest rows per testee:\n%s", countSQL)
	}
	if !strings.Contains(countSQL, "MAX(assessment.id) AS latest_id") || !strings.Contains(countSQL, "GROUP BY assessment.testee_id") {
		t.Fatalf("latest risk count query should derive latest per-testee risk rows:\n%s", countSQL)
	}
	if strings.Contains(countSQL, "NOT EXISTS") || strings.Contains(countSQL, "newer.id > a.id") {
		t.Fatalf("latest risk count query should not use anti-join shape:\n%s", countSQL)
	}
}

func TestLatestRiskQueueArgsMatchDerivedLatestSQLPlaceholders(t *testing.T) {
	restricted := latestRiskQueueArgs(workbenchreadmodel.LatestRiskQueueFilter{
		OrgID:               9,
		TesteeIDs:           []uint64{3002, 3001, 3001},
		RestrictToTesteeIDs: true,
		RiskLevels:          []string{"HIGH", "severe"},
	})
	if len(restricted) != 6 {
		t.Fatalf("restricted args = %#v, want 6 args", restricted)
	}
	if restricted[0] != int64(9) || restricted[2] != "evaluated" || restricted[3] != int64(9) || restricted[4] != "evaluated" {
		t.Fatalf("restricted args = %#v, want inner org/status then outer org/status", restricted)
	}
	if ids, ok := restricted[1].([]uint64); !ok || len(ids) != 2 || ids[0] != 3002 || ids[1] != 3001 {
		t.Fatalf("restricted testee ids = %#v, want unique ids in input order", restricted[1])
	}
	if risks, ok := restricted[5].([]string); !ok || len(risks) != 2 || risks[0] != "high" || risks[1] != "severe" {
		t.Fatalf("restricted risk args = %#v, want normalized risks", restricted[5])
	}

	allOrg := latestRiskQueueArgs(workbenchreadmodel.LatestRiskQueueFilter{OrgID: 9})
	if len(allOrg) != 5 || allOrg[0] != int64(9) || allOrg[1] != "evaluated" || allOrg[2] != int64(9) || allOrg[3] != "evaluated" {
		t.Fatalf("all-org args = %#v, want inner org/status then outer org/status", allOrg)
	}
	if risks, ok := allOrg[4].([]string); !ok || len(risks) != 2 || risks[0] != "high" || risks[1] != "severe" {
		t.Fatalf("all-org risk args = %#v, want default high/severe", allOrg[4])
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
		"ORDER BY COALESCE(assessment.evaluated_at, assessment.updated_at, assessment.created_at) DESC, assessment.id DESC",
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
