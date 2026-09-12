package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"time"
)

// scopedWindowMetrics aggregates published facts after current ownership
// selection. It must not read the company daily projections, which omit TesteeID.
func (s *ReadStore) scopedWindowMetrics(ctx context.Context, orgID int64, stores authz.StoreRange, from, to time.Time) (app.OverviewMetrics, error) {
	access := s.scopedFacts(ctx, orgID, stores, "statistics_access_fact").Where("f.stat_date>=? AND f.stat_date<?", from, to).Select(`
 COALESCE(SUM(fact_type='entry_opened'),0) entry_opened_count,
 COALESCE(SUM(fact_type='intake_confirmed'),0) intake_confirmed_count,
 COALESCE(SUM(fact_type='testee_created'),0) testee_created_count,
 COALESCE(SUM(fact_type='care_relationship_established'),0) care_relationship_established_count,
 COALESCE(SUM(fact_type='care_relationship_transferred'),0) care_relationship_transferred_count`)
	assessment := s.scopedFacts(ctx, orgID, stores, "statistics_assessment_fact").Where("f.stat_date>=? AND f.stat_date<?", from, to).Select(`
 COALESCE(SUM(fact_type='answersheet_submitted'),0) window_answer_sheet_submitted_count,
 COALESCE(SUM(fact_type='assessment_created'),0) window_assessment_created_count,
 COALESCE(SUM(fact_type='outcome_committed'),0) window_outcome_committed_count,
 COALESCE(SUM(fact_type='assessment_failed'),0) window_assessment_failed_count,
 COALESCE(SUM(fact_type='report_generated'),0) window_report_generated_count,
 COALESCE(SUM(fact_type='report_failed'),0) window_report_failed_count`)
	plan := s.scopedFacts(ctx, orgID, stores, "statistics_plan_fact").Where("f.stat_date>=? AND f.stat_date<?", from, to).Select(`
 COALESCE(SUM(fact_type='task_created'),0) task_created_count,
 COALESCE(SUM(fact_type='task_opened'),0) task_opened_count,
 COALESCE(SUM(fact_type='task_completed'),0) task_completed_count,
 COALESCE(SUM(fact_type='task_expired'),0) task_expired_count,
 COALESCE(SUM(fact_type='task_canceled'),0) task_canceled_count`)
	var result app.OverviewMetrics
	err := s.db.WithContext(ctx).Table("(?) AS access_counts", access).Joins("CROSS JOIN (?) AS assessment_counts", assessment).Joins("CROSS JOIN (?) AS plan_counts", plan).Select("access_counts.*, assessment_counts.*, plan_counts.*").Scan(&result).Error
	return result, err
}
