package statistics

import (
	"testing"
	"time"

	domainstats "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
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

func TestPlanStatsCacheKey(t *testing.T) {
	t.Parallel()

	if got := planStatsCacheKey(12, 1001); got != "plan:12:1001" {
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
