package readmodel

import (
	"testing"
	"time"

	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	statisticsreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticsreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
