package statistics

import (
	"testing"
	"time"

	domainstats "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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

func TestNormalizeQueryFilterWithExplicitRange(t *testing.T) {
	t.Parallel()

	got, err := normalizeQueryFilter(QueryFilter{
		Preset: "7d",
		From:   "2026-04-01",
		To:     "2026-04-02",
	})
	if err != nil {
		t.Fatalf("normalizeQueryFilter returned error: %v", err)
	}
	if got.From.Format("2006-01-02 15:04:05") != "2026-04-01 00:00:00" {
		t.Fatalf("from = %s, want 2026-04-01 00:00:00", got.From.Format("2006-01-02 15:04:05"))
	}
	if got.To.Format("2006-01-02 15:04:05") != "2026-04-03 00:00:00" {
		t.Fatalf("to = %s, want 2026-04-03 00:00:00", got.To.Format("2006-01-02 15:04:05"))
	}
}

func TestNormalizeQueryFilterRejectsInvalidExplicitRange(t *testing.T) {
	t.Parallel()

	_, err := normalizeQueryFilter(QueryFilter{
		From: "2026-04-05",
		To:   "2026-04-01",
	})
	if err == nil {
		t.Fatal("expected invalid range to fail")
	}
}

func TestFillMissingDailyCounts(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)
	got := fillMissingDailyCounts(from, to, []domainstats.DailyCount{
		{Date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Count: 2},
		{Date: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC), Count: 5},
	})

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[1].Date.Format("2006-01-02") != "2026-04-02" || got[1].Count != 0 {
		t.Fatalf("got[1] = %+v, want zero-filled 2026-04-02", got[1])
	}
	if got[2].Count != 5 {
		t.Fatalf("got[2].Count = %d, want 5", got[2].Count)
	}
}

func TestNormalizePageAndCalcTotalPages(t *testing.T) {
	t.Parallel()

	page, pageSize := normalizePage(0, 500)
	if page != 1 || pageSize != 100 {
		t.Fatalf("normalizePage(0, 500) = (%d, %d), want (1, 100)", page, pageSize)
	}
	if got := calcTotalPages(101, pageSize); got != 2 {
		t.Fatalf("calcTotalPages(101, %d) = %d, want 2", pageSize, got)
	}
}

func TestPtrMetaIDFromUint64(t *testing.T) {
	t.Parallel()

	if got := ptrMetaIDFromUint64(nil); got != nil {
		t.Fatalf("ptrMetaIDFromUint64(nil) = %v, want nil", got)
	}

	value := uint64(42)
	got := ptrMetaIDFromUint64(&value)
	if got == nil || *got != meta.FromUint64(42) {
		t.Fatalf("ptrMetaIDFromUint64(&42) = %v, want 42", got)
	}
}
