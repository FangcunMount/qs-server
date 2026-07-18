package readmodel

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type trackingAcquirer struct {
	err      error
	acquired int
	released int
}

func (a *trackingAcquirer) Acquire(ctx context.Context) (context.Context, func(), error) {
	a.acquired++
	if a.err != nil {
		return ctx, func() {}, a.err
	}
	return ctx, func() { a.released++ }, nil
}

func TestStatisticsReadModelLimiterReleasesOnSuccessAndSQLError(t *testing.T) {
	t.Parallel()

	successLimiter := &trackingAcquirer{}
	successModel := NewReadModel(newDryRunStatisticsReadModelDB(t), mysql.BaseRepositoryOptions{Limiter: successLimiter})
	if _, err := successModel.GetContentBatchTotals(context.Background(), 1, nil); err != nil {
		t.Fatalf("empty content totals: %v", err)
	}
	if successLimiter.acquired != 1 || successLimiter.released != 1 {
		t.Fatalf("success limiter acquired/released = %d/%d, want 1/1", successLimiter.acquired, successLimiter.released)
	}

	failedDB := newClosedStatisticsReadModelDB(t)
	failureLimiter := &trackingAcquirer{}
	failureModel := NewReadModel(failedDB, mysql.BaseRepositoryOptions{Limiter: failureLimiter})
	_, err := failureModel.GetContentBatchTotals(context.Background(), 1, []statisticsApp.ContentReference{{Type: "questionnaire", Code: "Q-1"}})
	if err == nil {
		t.Fatal("content totals error = nil, want closed database error")
	}
	if failureLimiter.acquired != 1 || failureLimiter.released != 1 {
		t.Fatalf("failure limiter acquired/released = %d/%d, want 1/1", failureLimiter.acquired, failureLimiter.released)
	}

	wantAcquireErr := errors.New("limited")
	blockedLimiter := &trackingAcquirer{err: wantAcquireErr}
	blockedModel := NewReadModel(nil, mysql.BaseRepositoryOptions{Limiter: blockedLimiter})
	if _, err := blockedModel.GetContentBatchTotals(context.Background(), 1, nil); !errors.Is(err, wantAcquireErr) {
		t.Fatalf("blocked content totals error = %v, want %v", err, wantAcquireErr)
	}
	if blockedLimiter.released != 0 {
		t.Fatalf("blocked limiter released = %d, want 0", blockedLimiter.released)
	}
}

func TestAssessmentServiceAnswerSheetScanAliasMatchesGORMNaming(t *testing.T) {
	t.Parallel()

	want := schema.NamingStrategy{}.ColumnName("", "AnswerSheetSubmittedCount")
	if assessmentServiceAnswerSheetSubmittedScanAlias != want {
		t.Fatalf("answersheet submitted scan alias = %q, want %q", assessmentServiceAnswerSheetSubmittedScanAlias, want)
	}
}

func TestClinicianSubjectFromPODocumentsMapperContract(t *testing.T) {
	t.Parallel()

	operatorID := uint64(7001)
	row := clinicianSubjectFromPO(actorInfra.ClinicianPO{
		AuditFields:   mysql.AuditFields{ID: meta.FromUint64(101)},
		OperatorID:    &operatorID,
		Name:          "Dr. Zhang",
		Department:    "儿童心理",
		Title:         "主治医师",
		ClinicianType: "psychiatrist",
		IsActive:      true,
	})

	if row.ID.Uint64() != 101 || row.OperatorID == nil || row.OperatorID.Uint64() != operatorID {
		t.Fatalf("unexpected clinician identity fields: %#v", row)
	}
	if row.Name != "Dr. Zhang" || row.Department != "儿童心理" || row.Title != "主治医师" || row.ClinicianType != "psychiatrist" || !row.IsActive {
		t.Fatalf("unexpected clinician display fields: %#v", row)
	}
}

func TestAssessmentEntryMetaFromPODocumentsMapperContract(t *testing.T) {
	t.Parallel()

	version := "v1"
	expiresAt := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	createdAt := expiresAt.Add(-time.Hour)
	row := assessmentEntryMetaFromPO(actorInfra.AssessmentEntryPO{
		AuditFields:   mysql.AuditFields{ID: meta.FromUint64(201), CreatedAt: createdAt},
		OrgID:         9,
		ClinicianID:   meta.FromUint64(101),
		Token:         "entry-token",
		TargetType:    "scale",
		TargetCode:    "SDS",
		TargetVersion: &version,
		IsActive:      true,
		ExpiresAt:     &expiresAt,
	})

	if row.ID.Uint64() != 201 || row.OrgID != 9 || row.ClinicianID.Uint64() != 101 {
		t.Fatalf("unexpected entry identity fields: %#v", row)
	}
	if row.Token != "entry-token" || row.TargetType != "scale" || row.TargetCode != "SDS" || row.TargetVersion != "v1" || !row.IsActive {
		t.Fatalf("unexpected entry target fields: %#v", row)
	}
	if !row.CreatedAt.Equal(createdAt) || row.ExpiresAt == nil || !row.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected entry time fields: %#v", row)
	}
}

func TestBuildClinicianSubjectQueryDocumentsFilterPageOrderContract(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	var rows []actorInfra.ClinicianPO

	stmt := buildClinicianSubjectQuery(db.Session(&gorm.Session{DryRun: true}), 9).
		Order("id DESC").
		Offset(20).
		Limit(10).
		Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"org_id = ?",
		"deleted_at IS NULL",
		"ORDER BY id DESC",
		"LIMIT ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	if !containsStatisticsReadModelVar(stmt.Vars, int64(9)) || !containsStatisticsReadModelVar(stmt.Vars, 10) {
		t.Fatalf("query vars = %#v, want org/page limit", stmt.Vars)
	}
}

func TestBuildAssessmentEntryMetaQueryDocumentsClinicianAndActiveFilters(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	var rows []actorInfra.AssessmentEntryPO
	clinicianID := uint64(101)
	activeOnly := true

	stmt := buildAssessmentEntryMetaQuery(db.Session(&gorm.Session{DryRun: true}), 9, &clinicianID, &activeOnly).
		Order("id DESC").
		Offset(0).
		Limit(20).
		Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"org_id = ?",
		"deleted_at IS NULL",
		"clinician_id = ?",
		"is_active = ?",
		"ORDER BY id DESC",
		"LIMIT ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{int64(9), clinicianID, activeOnly, 20} {
		if !containsStatisticsReadModelVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
	}
}

func TestBuildContentBatchTotalsQueriesPreserveTypedIdentityAndOrgScope(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	var rows []statisticsApp.ContentBatchTotal
	stmt := buildQuestionnaireContentBatchTotalsQuery(db.Session(&gorm.Session{DryRun: true}), 9, []string{"COMMON"}).
		Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"'questionnaire' AS type",
		"questionnaire_code AS code",
		"COUNT(*) AS total_submissions",
		"status = 'evaluated'",
		"org_id = ?",
		"evaluation_model_kind <> 'scale'",
		"deleted_at IS NULL",
		"questionnaire_code IN",
		"GROUP BY `questionnaire_code`",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{int64(9), "COMMON"} {
		if !containsStatisticsReadModelVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
	}

	realtimeStmt := buildScaleContentBatchTotalsQuery(db.Session(&gorm.Session{DryRun: true}), 9, []string{"COMMON"}).
		Find(&rows).Statement
	realtimeSQL := realtimeStmt.SQL.String()
	for _, token := range []string{"'scale' AS type", "FROM `assessment`", "org_id = ?", "status = 'evaluated'", "evaluation_model_kind = 'scale'", "evaluation_model_code", "questionnaire_code", "GROUP BY"} {
		if !strings.Contains(realtimeSQL, token) {
			t.Fatalf("realtime query sql %q does not contain %q", realtimeSQL, token)
		}
	}
	for _, want := range []interface{}{int64(9), "COMMON"} {
		if !containsStatisticsReadModelVar(realtimeStmt.Vars, want) {
			t.Fatalf("realtime query vars = %#v, want %v", realtimeStmt.Vars, want)
		}
	}
}

func TestBuildOrganizationCumulativeQueriesUseAssessmentFactsAndLocalDayBounds(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	today := time.Date(2026, 7, 18, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	tomorrow := today.AddDate(0, 0, 1)
	var submissions struct{ Total, Today int64 }
	stmt := buildOrganizationAnswerSheetSubmissionsQuery(db.Session(&gorm.Session{DryRun: true}), 9, today, tomorrow).
		Scan(&submissions).Statement
	sql := stmt.SQL.String()
	for _, token := range []string{"FROM `assessment`", "submitted_at IS NOT NULL", "submitted_at >= ?", "submitted_at < ?", "org_id = ?", "deleted_at IS NULL"} {
		if !strings.Contains(sql, token) {
			t.Fatalf("submission query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{today, tomorrow, int64(9)} {
		if !containsStatisticsReadModelVar(stmt.Vars, want) {
			t.Fatalf("submission query vars = %#v, want %v", stmt.Vars, want)
		}
	}

	var content struct{ Count int64 }
	contentStmt := buildOrganizationContentCountQuery(db.Session(&gorm.Session{DryRun: true}), 9).Scan(&content).Statement
	contentSQL := contentStmt.SQL.String()
	for _, token := range []string{"FROM assessment", "SELECT DISTINCT", "evaluation_model_kind = 'scale'", "questionnaire_code", "org_id = ?", "deleted_at IS NULL"} {
		if !strings.Contains(contentSQL, token) {
			t.Fatalf("content count query sql %q does not contain %q", contentSQL, token)
		}
	}
	if !containsStatisticsReadModelVar(contentStmt.Vars, int64(9)) {
		t.Fatalf("content count vars = %#v, want org 9", contentStmt.Vars)
	}
}

func TestBuildPlanTaskTrendQueryDocumentsDatePlanAndOrderContract(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	var rows []struct {
		StatDate time.Time
		Count    int64
	}
	planID := uint64(501)
	from := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)

	stmt := buildPlanTaskTrendQuery(db.Session(&gorm.Session{DryRun: true}), 9, &planID, from, to).
		Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"stat_date",
		"SUM(task_created_count)",
		"SUM(task_opened_count)",
		"SUM(task_completed_count)",
		"SUM(task_expired_count)",
		"org_id = ?",
		"stat_date >= ?",
		"stat_date < ?",
		"deleted_at IS NULL",
		"GROUP BY",
		"ORDER BY stat_date ASC",
		"plan_id = ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	if !containsStatisticsReadModelVar(stmt.Vars, int64(9)) || !containsStatisticsReadModelVar(stmt.Vars, planID) {
		t.Fatalf("query vars = %#v, want org/plan", stmt.Vars)
	}
}

func TestBuildPlanTaskFulfillmentWindowQueryDocumentsCohortContract(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	var row struct {
		PlannedTaskCount int64
	}
	planID := uint64(501)
	from := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)
	now := to.AddDate(0, 0, 1)

	stmt := buildPlanTaskFulfillmentWindowQuery(db.Session(&gorm.Session{DryRun: true}), 9, &planID, from, to, now).
		Scan(&row).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"assessment_task t",
		"JOIN assessment_plan p",
		"FORCE INDEX (idx_task_org_deleted_planned_status)",
		"FORCE INDEX (idx_task_org_deleted_expire_status)",
		"CROSS JOIN",
		"t.planned_at >= ?",
		"t.expire_at IS NOT NULL",
		"t.completed_at <= t.expire_at",
		"t.completed_at > t.expire_at",
		"t.expire_at < ?",
		"t.status <> ?",
		"t.plan_id = ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{int64(9), planID, "completed", "expired", "canceled"} {
		if !containsStatisticsReadModelVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
	}
}

func TestBuildPlanTaskFulfillmentTrendQueryDocumentsCohortDateContract(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	var rows []struct {
		StatDate time.Time
	}
	planID := uint64(501)
	from := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)

	stmt := buildPlanTaskFulfillmentTrendQuery(db.Session(&gorm.Session{DryRun: true}), 9, &planID, from, to, to).
		Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"DATE(t.planned_at)",
		"DATE(t.expire_at)",
		"FORCE INDEX (idx_task_org_deleted_planned_status)",
		"FORCE INDEX (idx_task_org_deleted_expire_status)",
		"planned_task_count",
		"due_task_count",
		"completed_task_count",
		"overdue_task_count",
		"UNION ALL",
		"ORDER BY raw.stat_date ASC",
		"t.plan_id = ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{int64(9), planID, "completed", "expired", "canceled"} {
		if !containsStatisticsReadModelVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
	}
}

func TestBuildPlanTaskDistinctTesteeCountQueryDocumentsRangeAndPreGroupContract(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	var row struct {
		Count int64
	}
	planID := uint64(501)
	from := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)

	query, err := buildPlanTaskDistinctTesteeCountQuery(db.Session(&gorm.Session{DryRun: true}), 9, &planID, "created_at", "", from, to)
	if err != nil {
		t.Fatalf("build created_at distinct query: %v", err)
	}
	stmt := query.Scan(&row).Statement
	sql := stmt.SQL.String()
	for _, token := range []string{
		"COUNT(DISTINCT scoped.testee_id)",
		"FORCE INDEX (idx_task_org_deleted_created)",
		"t.created_at >= ?",
		"t.created_at < ?",
		"GROUP BY t.plan_id, t.testee_id",
		"JOIN assessment_plan p",
		"t.plan_id = ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("created_at distinct query sql %q does not contain %q", sql, token)
		}
	}
	if !containsStatisticsReadModelVar(stmt.Vars, int64(9)) || !containsStatisticsReadModelVar(stmt.Vars, planID) {
		t.Fatalf("query vars = %#v, want org/plan", stmt.Vars)
	}

	query, err = buildPlanTaskDistinctTesteeCountQuery(db.Session(&gorm.Session{DryRun: true}), 9, nil, "completed_at", "completed", from, to)
	if err != nil {
		t.Fatalf("build completed_at distinct query: %v", err)
	}
	stmt = query.Scan(&row).Statement
	sql = stmt.SQL.String()
	for _, token := range []string{
		"FORCE INDEX (idx_task_org_deleted_completed_status)",
		"t.completed_at >= ?",
		"t.status = ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("completed_at distinct query sql %q does not contain %q", sql, token)
		}
	}
	if !containsStatisticsReadModelVar(stmt.Vars, "completed") {
		t.Fatalf("query vars = %#v, want completed status", stmt.Vars)
	}
}

func TestPlanTaskFulfillmentWindowFromRowCalculatesRatesOnDueCohort(t *testing.T) {
	t.Parallel()

	got := planTaskFulfillmentWindowFromRow(12, 10, 7, 6, 2)
	if got.PlannedTaskCount != 12 || got.DueTaskCount != 10 || got.CompletedTaskCount != 7 || got.OnTimeCompletedCount != 6 || got.OverdueTaskCount != 2 {
		t.Fatalf("unexpected fulfillment window: %+v", got)
	}
	if got.CompletionRate != 70 || got.OnTimeCompletionRate != 60 {
		t.Fatalf("rates = %.2f/%.2f, want 70/60", got.CompletionRate, got.OnTimeCompletionRate)
	}
}

func newDryRunStatisticsReadModelDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	return db
}

func newClosedStatisticsReadModelDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/qs_server_closed?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("open closed sql db: %v", err)
	}
	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{
		Conn:                      conn,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open closed gorm db: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close sql db: %v", err)
	}
	return db
}

func containsStatisticsReadModelVar(values []interface{}, want interface{}) bool {
	for _, value := range values {
		if value == want {
			return true
		}
		if nested, ok := value.([]string); ok {
			for _, item := range nested {
				if item == want {
					return true
				}
			}
		}
	}
	return false
}
