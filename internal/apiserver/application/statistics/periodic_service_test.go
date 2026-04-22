package statistics

import (
	"testing"
	"time"

	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
)

func TestBuildPeriodicProjectStatisticsBuildsOrderedSummary(t *testing.T) {
	completedAt := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	expireAt := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	assessmentID := uint64(2001)

	project, hasActiveTask := buildPeriodicProjectStatistics("1001", []planInfra.AssessmentTaskPO{
		{
			PlanID:       1001,
			Seq:          2,
			Status:       "pending",
			ScaleCode:    "scale-b",
			PlannedAt:    time.Date(2026, 4, 17, 8, 0, 0, 0, time.UTC),
			ExpireAt:     &expireAt,
			AssessmentID: nil,
		},
		{
			PlanID:       1001,
			Seq:          1,
			Status:       "completed",
			ScaleCode:    "scale-a",
			PlannedAt:    time.Date(2026, 4, 10, 8, 0, 0, 0, time.UTC),
			CompletedAt:  &completedAt,
			AssessmentID: &assessmentID,
		},
	}, map[uint64]string{
		assessmentID: "PHQ-9",
	})

	if !hasActiveTask {
		t.Fatalf("expected pending task to mark project active")
	}
	if project.ScaleName != "PHQ-9" || project.ProjectName != "PHQ-9" {
		t.Fatalf("expected assessment name to win, got %+v", project)
	}
	if project.TotalWeeks != 2 || project.CompletedWeeks != 1 {
		t.Fatalf("unexpected week counts: %+v", project)
	}
	if project.CurrentWeek != 2 {
		t.Fatalf("expected current week 2, got %d", project.CurrentWeek)
	}
	if project.CompletionRate != 50 {
		t.Fatalf("expected completion rate 50, got %v", project.CompletionRate)
	}
	if len(project.Tasks) != 2 || project.Tasks[0].Week != 1 || project.Tasks[1].Week != 2 {
		t.Fatalf("expected tasks to be ordered by sequence, got %+v", project.Tasks)
	}
	if project.StartDate == nil || project.StartDate.Format(time.RFC3339) != "2026-04-10T08:00:00Z" {
		t.Fatalf("unexpected start date: %+v", project.StartDate)
	}
	if project.EndDate == nil || project.EndDate.Format(time.RFC3339) != "2026-04-17T09:00:00Z" {
		t.Fatalf("unexpected end date: %+v", project.EndDate)
	}
}
