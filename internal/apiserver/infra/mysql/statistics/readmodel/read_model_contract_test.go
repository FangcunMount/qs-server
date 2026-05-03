package readmodel

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	statisticsreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticsreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestStatisticsTrendMetricMappingsDocumentColumnContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		ok   bool
		want string
	}{
		{
			name: "overview assessment created",
			got:  mustOverviewTrendField(statisticsreadmodel.OrgOverviewMetricAssessmentCreated),
			ok:   true,
			want: "assessment_created_count",
		},
		{
			name: "access entry opened",
			got:  mustAccessFunnelTrendField(statisticsreadmodel.AccessFunnelMetricEntryOpened),
			ok:   true,
			want: "access_entry_opened_count",
		},
		{
			name: "assessment report generated",
			got:  mustAssessmentServiceTrendField(statisticsreadmodel.AssessmentServiceMetricReportGenerated),
			ok:   true,
			want: "service_report_generated_count",
		},
		{
			name: "plan task completed",
			got:  mustPlanTaskTrendField(statisticsreadmodel.PlanTaskMetricCompleted),
			ok:   true,
			want: "task_completed_count",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !tc.ok || tc.got != tc.want {
				t.Fatalf("field = %q ok=%v, want %q", tc.got, tc.ok, tc.want)
			}
		})
	}
	if field, ok := overviewTrendField(statisticsreadmodel.OrgOverviewMetric("unknown")); ok || field != "" {
		t.Fatalf("unknown overview metric field=%q ok=%v, want empty false", field, ok)
	}
	if field, ok := planTaskTrendField(statisticsreadmodel.PlanTaskMetric("unknown")); ok || field != "" {
		t.Fatalf("unknown plan metric field=%q ok=%v, want empty false", field, ok)
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

func TestBuildQuestionnaireBatchTotalsQueryDocumentsDedupedTotalsContract(t *testing.T) {
	t.Parallel()

	db := newDryRunStatisticsReadModelDB(t)
	var rows []statisticsreadmodel.QuestionnaireBatchTotal

	stmt := buildQuestionnaireBatchTotalsQuery(db.Session(&gorm.Session{DryRun: true}), 9, []string{"Q-A", "Q-B"}).
		Find(&rows).Statement

	sql := stmt.SQL.String()
	for _, token := range []string{
		"content_code AS code",
		"SUM(submission_count)",
		"SUM(completion_count)",
		"org_id = ?",
		"content_type = ?",
		"deleted_at IS NULL",
		"content_code IN",
		"GROUP BY",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("query sql %q does not contain %q", sql, token)
		}
	}
	for _, want := range []interface{}{int64(9), "questionnaire", "Q-A", "Q-B"} {
		if !containsStatisticsReadModelVar(stmt.Vars, want) {
			t.Fatalf("query vars = %#v, want %v", stmt.Vars, want)
		}
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

func mustOverviewTrendField(metric statisticsreadmodel.OrgOverviewMetric) string {
	field, _ := overviewTrendField(metric)
	return field
}

func mustAccessFunnelTrendField(metric statisticsreadmodel.AccessFunnelMetric) string {
	field, _ := accessFunnelTrendField(metric)
	return field
}

func mustAssessmentServiceTrendField(metric statisticsreadmodel.AssessmentServiceMetric) string {
	field, _ := assessmentServiceTrendField(metric)
	return field
}

func mustPlanTaskTrendField(metric statisticsreadmodel.PlanTaskMetric) string {
	field, _ := planTaskTrendField(metric)
	return field
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
