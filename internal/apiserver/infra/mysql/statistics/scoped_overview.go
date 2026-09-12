package statistics

import (
	"context"
	"database/sql"
	"fmt"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"gorm.io/gorm"
	"time"
)

// ScopedOverview reads all components from one repeatable-read snapshot.
// cutoff is the published statistics cutoff; caller must verify publication
// identity again before returning. Results never enter the company-wide cache.
func (s *ReadStore) ScopedOverview(ctx context.Context, orgID int64, stores authz.StoreRange, from, to, cutoff time.Time) (app.ScopedOverviewData, error) {
	var result app.ScopedOverviewData
	if orgID <= 0 || !from.Before(to) || cutoff.IsZero() {
		return result, fmt.Errorf("invalid scoped statistics query")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return result, err
	}
	defer release()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reader := &ReadStore{db: tx}
		population, err := reader.scopedPopulationMetrics(ctx, orgID, stores)
		if err != nil {
			return err
		}
		window, err := reader.scopedWindowMetrics(ctx, orgID, stores, from, to)
		if err != nil {
			return err
		}
		trends, err := reader.scopedActivityTrends(ctx, orgID, stores, from, to)
		if err != nil {
			return err
		}
		fulfillment, err := reader.scopedFulfillment(ctx, orgID, stores, from, to, cutoff)
		if err != nil {
			return err
		}
		window.TesteeCount = population.TesteeCount
		window.ClinicianCount = population.ClinicianCount
		window.ActiveClinicianCount = population.ActiveClinicianCount
		window.EntryCount = population.EntryCount
		window.ActiveEntryCount = population.ActiveEntryCount
		window.ActiveEnrollmentCount = population.ActiveEnrollmentCount
		window.AnswerSheetSubmissionCount = population.AnswerSheetSubmissionCount
		window.AssessmentCount = population.AssessmentCount
		window.ReportCount = population.ReportCount
		window.ContentCount = population.ContentCount
		days := map[string]scopedFulfillmentDay{}
		for _, row := range fulfillment {
			// MySQL DATE scans can include a time suffix depending on driver settings.
			if len(row.CohortDate) < 10 {
				return fmt.Errorf("invalid fulfillment cohort date")
			}
			key := row.CohortDate[:10]
			if _, err := time.Parse("2006-01-02", key); err != nil {
				return err
			}
			days[key] = row
			window.PlannedTaskCount += row.Planned
			window.DueTaskCount += row.Due
			window.CompletedOnTimeCount += row.CompletedOnTime
			window.CompletedOverdueCount += row.CompletedOverdue
			window.UncompletedOverdueCount += row.UncompletedOverdue
		}
		for date := from; date.Before(to); date = date.AddDate(0, 0, 1) {
			row := days[date.Format("2006-01-02")]
			trends.PlanFulfillment.Planned = append(trends.PlanFulfillment.Planned, daily(date, row.Planned))
			trends.PlanFulfillment.Due = append(trends.PlanFulfillment.Due, daily(date, row.Due))
			trends.PlanFulfillment.Completed = append(trends.PlanFulfillment.Completed, daily(date, row.CompletedOnTime+row.CompletedOverdue))
			trends.PlanFulfillment.Overdue = append(trends.PlanFulfillment.Overdue, daily(date, row.CompletedOverdue+row.UncompletedOverdue))
		}
		result = app.ScopedOverviewData{Metrics: window, Trends: trends}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return app.ScopedOverviewData{}, err
	}
	return result, nil
}
