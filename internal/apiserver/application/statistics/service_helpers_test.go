package statistics

import (
	"testing"
	"time"

	domainstats "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
)

func TestCurrentDayBounds(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 15, 4, 5, 0, time.FixedZone("CST", 8*3600))
	start, end := currentDayBounds(now)
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Fatalf("start = %v, want midnight", start)
	}
	if !end.Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("end = %v, want %v", end, start.AddDate(0, 0, 1))
	}
}

func TestQuestionnaireStatsCacheKey(t *testing.T) {
	t.Parallel()

	if got := questionnaireStatsCacheKey(12, "PHQ9"); got != "questionnaire:12:PHQ9" {
		t.Fatalf("cache key = %q", got)
	}
}

func TestDaysAgoReturnsApproximateOffset(t *testing.T) {
	t.Parallel()

	got := daysAgo(7)
	if got == nil {
		t.Fatal("daysAgo returned nil")
	}
	diff := time.Since(*got)
	if diff < 7*24*time.Hour || diff > 8*24*time.Hour {
		t.Fatalf("daysAgo(7) diff = %v, want within [7d, 8d]", diff)
	}
}

func TestConvertAccumulatedPOToQuestionnaireStatistics(t *testing.T) {
	t.Parallel()

	service := &questionnaireStatisticsService{
		aggregator: domainstats.NewAggregator(),
	}
	stats := service.convertAccumulatedPOToQuestionnaireStatistics(&statisticsinfra.StatisticsAccumulatedPO{
		TotalSubmissions:   20,
		TotalCompletions:   15,
		Last7dSubmissions:  3,
		Last15dSubmissions: 8,
		Last30dSubmissions: 10,
		Distribution: statisticsinfra.JSONField{
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

func TestConvertPlanPOToPlanStatistics(t *testing.T) {
	t.Parallel()

	service := &planStatisticsService{
		aggregator: domainstats.NewAggregator(),
	}
	stats := service.convertPlanPOToPlanStatistics(&statisticsinfra.StatisticsPlanPO{
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

func TestConvertAccumulatedPOToSystemStatistics(t *testing.T) {
	t.Parallel()

	service := &systemStatisticsService{
		aggregator: domainstats.NewAggregator(),
	}
	stats := service.convertAccumulatedPOToSystemStatistics(&statisticsinfra.StatisticsAccumulatedPO{
		TotalSubmissions: 25,
		Distribution: statisticsinfra.JSONField{
			"status": map[string]interface{}{
				"interpreted": float64(20),
				"pending":     float64(5),
			},
			"questionnaire_count":     float64(6),
			"answer_sheet_count":      float64(8),
			"testee_count":            float64(10),
			"today_new_assessments":   float64(2),
			"today_new_answer_sheets": float64(3),
			"today_new_testees":       float64(1),
		},
	}, 7)

	if stats.AssessmentCount != 25 || stats.OrgID != 7 {
		t.Fatalf("unexpected system stats: %+v", stats)
	}
	if stats.AssessmentStatusDistribution["interpreted"] != 20 || stats.AssessmentStatusDistribution["pending"] != 5 {
		t.Fatalf("unexpected status distribution: %+v", stats.AssessmentStatusDistribution)
	}
	if stats.QuestionnaireCount != 6 || stats.AnswerSheetCount != 8 || stats.TesteeCount != 10 {
		t.Fatalf("unexpected distribution counts: %+v", stats)
	}
	if stats.TodayNewAssessments != 2 || stats.TodayNewAnswerSheets != 3 || stats.TodayNewTestees != 1 {
		t.Fatalf("unexpected realtime fields: %+v", stats)
	}
}
