package statistics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

type repositoryTrackingAcquirer struct {
	acquired int
	released int
}

func (a *repositoryTrackingAcquirer) Acquire(ctx context.Context) (context.Context, func(), error) {
	a.acquired++
	return ctx, func() { a.released++ }, nil
}

func TestStatisticsRepositoryLimiterReleasesWhenTransactionIsRejected(t *testing.T) {
	t.Parallel()

	limiter := &repositoryTrackingAcquirer{}
	repo := NewStatisticsRepository(nil, mysql.BaseRepositoryOptions{Limiter: limiter})
	err := repo.RebuildDailyStatistics(context.Background(), 1, time.Now(), time.Now().AddDate(0, 0, 1))
	if err == nil {
		t.Fatal("RebuildDailyStatistics error = nil, want missing transaction error")
	}
	if limiter.acquired != 1 || limiter.released != 1 {
		t.Fatalf("limiter acquired/released = %d/%d, want 1/1", limiter.acquired, limiter.released)
	}
}

func TestAccessFunnelInsertSQLUsesIntakeLogFacts(t *testing.T) {
	for _, token := range []string{
		"GREATEST(SUM(raw.entry_opened_count), SUM(raw.intake_confirmed_count)), SUM(raw.intake_confirmed_count)",
		"FROM assessment_entry_intake_log WHERE org_id = ? AND deleted_at IS NULL AND intake_at >= ? AND intake_at < ?",
		"FROM assessment_entry_intake_log WHERE org_id = ? AND deleted_at IS NULL AND testee_created = 1",
		"FROM assessment_entry_intake_log WHERE org_id = ? AND deleted_at IS NULL AND assignment_created = 1",
	} {
		if !strings.Contains(accessFunnelOrgInsertSQL, token) {
			t.Fatalf("access funnel SQL does not contain %q", token)
		}
	}
	for _, token := range []string{
		"FROM testee WHERE",
		"FROM clinician_relation WHERE",
	} {
		if strings.Contains(accessFunnelOrgInsertSQL, token) {
			t.Fatalf("access funnel SQL must use intake-log facts, found %q", token)
		}
	}
}
