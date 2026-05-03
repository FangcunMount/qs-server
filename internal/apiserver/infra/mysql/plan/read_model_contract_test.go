package plan

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestPlanPOToReadRowMapsReadModelFields(t *testing.T) {
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	row := planRowFromPO(&AssessmentPlanPO{
		AuditFields:  mysql.AuditFields{ID: meta.FromUint64(101), CreatedAt: now, UpdatedAt: now},
		OrgID:        9,
		ScaleCode:    "SDS",
		ScheduleType: "by_week",
		TriggerTime:  "19:00:00",
		Interval:     2,
		TotalTimes:   4,
		Status:       "active",
	})

	if row.ID != 101 || row.OrgID != 9 || row.ScaleCode != "SDS" || row.ScheduleType != "by_week" {
		t.Fatalf("unexpected plan row: %#v", row)
	}
	if row.TriggerTime != "19:00:00" || row.Interval != 2 || row.TotalTimes != 4 || row.Status != "active" {
		t.Fatalf("unexpected plan schedule/status fields: %#v", row)
	}
}

func TestTaskPOToReadRowMapsReadModelFields(t *testing.T) {
	plannedAt := time.Date(2026, 5, 3, 9, 0, 0, 0, time.UTC)
	openAt := plannedAt.Add(time.Hour)
	expireAt := openAt.Add(24 * time.Hour)
	completedAt := openAt.Add(2 * time.Hour)
	assessmentID := uint64(7001)

	row := taskRowFromPO(&AssessmentTaskPO{
		AuditFields:  mysql.AuditFields{ID: meta.FromUint64(201), CreatedAt: plannedAt, UpdatedAt: openAt},
		PlanID:       101,
		Seq:          2,
		OrgID:        9,
		TesteeID:     3001,
		ScaleCode:    "SDS",
		PlannedAt:    plannedAt,
		OpenAt:       &openAt,
		ExpireAt:     &expireAt,
		CompletedAt:  &completedAt,
		Status:       "completed",
		AssessmentID: &assessmentID,
		EntryToken:   "token",
		EntryURL:     "https://entry.example.com",
	})

	if row.ID != 201 || row.PlanID != 101 || row.Seq != 2 || row.OrgID != 9 || row.TesteeID != 3001 {
		t.Fatalf("unexpected task identity fields: %#v", row)
	}
	if row.ScaleCode != "SDS" || row.Status != "completed" || row.AssessmentID == nil || *row.AssessmentID != assessmentID {
		t.Fatalf("unexpected task status/assessment fields: %#v", row)
	}
	if row.OpenAt == nil || !row.OpenAt.Equal(openAt) || row.ExpireAt == nil || !row.ExpireAt.Equal(expireAt) {
		t.Fatalf("unexpected task time fields: %#v", row)
	}
}

func TestBuildPlanListQueryDocumentsFilterContract(t *testing.T) {
	db := newDryRunPlanDB(t)
	var rows []AssessmentPlanPO

	stmt := buildPlanListQuery(
		db.Session(&gorm.Session{DryRun: true}).Model(&AssessmentPlanPO{}),
		planreadmodel.PlanFilter{OrgID: 9, ScaleCode: "SDS", Status: "active"},
	).Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"deleted_at IS NULL",
		"org_id = ?",
		"scale_code = ?",
		"status = ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{int64(9), "SDS", "active"} {
		if !containsPlanVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
	}
}

func TestBuildTaskListQueryDocumentsAccessScopeContract(t *testing.T) {
	db := newDryRunPlanDB(t)
	var rows []AssessmentTaskPO
	planID := uint64(101)
	status := "opened"

	stmt := buildTaskListQuery(
		db.Session(&gorm.Session{DryRun: true}).Model(&AssessmentTaskPO{}),
		planreadmodel.TaskFilter{
			OrgID:                 9,
			PlanID:                &planID,
			Status:                &status,
			RestrictToAccessScope: true,
			AccessibleTesteeIDs:   []uint64{3001, 3002},
		},
	).Order("planned_at DESC").Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"org_id = ?",
		"deleted_at IS NULL",
		"plan_id = ?",
		"testee_id IN",
		"status = ?",
		"ORDER BY planned_at DESC",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	if !containsPlanVar(stmt.Vars, int64(9)) || !containsPlanVar(stmt.Vars, planID) || !containsPlanVar(stmt.Vars, "opened") {
		t.Fatalf("query vars = %#v, want org/plan/status", stmt.Vars)
	}
}

func TestBuildTaskWindowQueryDocumentsCursorOrderAndLimitContract(t *testing.T) {
	db := newDryRunPlanDB(t)
	var rows []AssessmentTaskPO
	status := "opened"
	plannedBefore := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)

	stmt := buildTaskWindowQuery(
		db.Session(&gorm.Session{DryRun: true}).Model(&AssessmentTaskPO{}),
		planreadmodel.TaskWindowFilter{
			OrgID:         9,
			PlanID:        101,
			TesteeIDs:     []uint64{3001, 3002},
			Status:        &status,
			PlannedBefore: &plannedBefore,
		},
	).Order("planned_at ASC").Order("id ASC").Limit(11).Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"org_id = ?",
		"plan_id = ?",
		"deleted_at IS NULL",
		"testee_id IN",
		"status = ?",
		"planned_at <= ?",
		"ORDER BY planned_at ASC,id ASC",
		"LIMIT ?",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	if !containsPlanVar(stmt.Vars, int64(9)) || !containsPlanVar(stmt.Vars, uint64(101)) || !containsPlanVar(stmt.Vars, "opened") || !containsPlanVar(stmt.Vars, 11) {
		t.Fatalf("query vars = %#v, want org/plan/status/limit", stmt.Vars)
	}
}

func newDryRunPlanDB(t *testing.T) *gorm.DB {
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

func containsPlanVar(values []interface{}, want interface{}) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
