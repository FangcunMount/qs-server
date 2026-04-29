package statistics

import (
	"context"
	"testing"
	"time"

	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
)

func TestAccumulatedPOToQuestionnaireStatistics(t *testing.T) {
	t.Parallel()

	repo := &StatisticsRepository{}
	stats := repo.accumulatedPOToQuestionnaireStatistics(&StatisticsAccumulatedPO{
		TotalSubmissions:   20,
		TotalCompletions:   15,
		Last7dSubmissions:  3,
		Last15dSubmissions: 8,
		Last30dSubmissions: 10,
		Distribution: JSONField{
			"origin": map[string]interface{}{
				"entry": float64(11),
				"plan":  float64(9),
			},
		},
	}, 9, "PHQ9")

	if stats.OrgID != 9 || stats.QuestionnaireCode != "PHQ9" {
		t.Fatalf("unexpected identity fields: %+v", stats)
	}
	if stats.TotalSubmissions != 20 || stats.TotalCompletions != 15 {
		t.Fatalf("unexpected totals: %+v", stats)
	}
	if stats.OriginDistribution["entry"] != 11 || stats.OriginDistribution["plan"] != 9 {
		t.Fatalf("unexpected origin distribution: %+v", stats.OriginDistribution)
	}
	if stats.CompletionRate != 75 {
		t.Fatalf("completion rate = %v, want 75", stats.CompletionRate)
	}
}

func TestPlanPOToPlanStatistics(t *testing.T) {
	t.Parallel()

	repo := &StatisticsRepository{}
	stats := repo.planPOToPlanStatistics(context.Background(), &StatisticsPlanPO{
		OrgID:           3,
		PlanID:          1001,
		TotalTasks:      10,
		CompletedTasks:  4,
		PendingTasks:    5,
		ExpiredTasks:    1,
		EnrolledTestees: 8,
		ActiveTestees:   4,
	})

	if stats.PlanID != 1001 || stats.OrgID != 3 {
		t.Fatalf("unexpected identity fields: %+v", stats)
	}
	if stats.CompletionRate != 40 {
		t.Fatalf("completion rate = %v, want 40", stats.CompletionRate)
	}
	if stats.PendingTasks != 5 || stats.ExpiredTasks != 1 {
		t.Fatalf("unexpected plan stats: %+v", stats)
	}
}

func TestBuildPeriodicProjectStatisticsBuildsOrderedSummary(t *testing.T) {
	t.Parallel()

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
