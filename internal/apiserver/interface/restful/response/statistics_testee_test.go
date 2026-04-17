package response

import (
	"testing"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func TestNewTesteeStatisticsResponseFormatsAssessmentDates(t *testing.T) {
	first := time.Date(2026, 4, 1, 9, 30, 0, 0, time.Local)
	last := time.Date(2026, 4, 17, 15, 45, 8, 0, time.Local)

	resp := NewTesteeStatisticsResponse(&domainStatistics.TesteeStatistics{
		OrgID:                1,
		TesteeID:             123,
		TotalAssessments:     10,
		CompletedAssessments: 8,
		PendingAssessments:   2,
		RiskDistribution: map[string]int64{
			"high": 3,
		},
		FirstAssessmentDate: &first,
		LastAssessmentDate:  &last,
	})

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.FirstAssessmentDate == nil || *resp.FirstAssessmentDate != "2026-04-01 09:30:00" {
		t.Fatalf("unexpected first_assessment_date: %+v", resp.FirstAssessmentDate)
	}
	if resp.LastAssessmentDate == nil || *resp.LastAssessmentDate != "2026-04-17 15:45:08" {
		t.Fatalf("unexpected last_assessment_date: %+v", resp.LastAssessmentDate)
	}
}
