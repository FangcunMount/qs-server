package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"time"
)

// scopedActivityTrends covers event-date series; fulfillment is a separate
// due-date/schedule contract and must not be approximated from completion events.
func (s *ReadStore) scopedActivityTrends(ctx context.Context, orgID int64, stores authz.StoreRange, from, to time.Time) (app.OverviewTrends, error) {
	counts := map[string]map[string]int64{}
	for _, table := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact"} {
		var rows []struct {
			StatDate time.Time
			FactType string
			Total    int64
		}
		err := s.scopedFacts(ctx, orgID, stores, table).Where("f.stat_date>=? AND f.stat_date<?", from, to).Select("f.stat_date,f.fact_type,COUNT(*) total").Group("f.stat_date,f.fact_type").Scan(&rows).Error
		if err != nil {
			return app.OverviewTrends{}, err
		}
		for _, row := range rows {
			date := row.StatDate.Format("2006-01-02")
			if counts[date] == nil {
				counts[date] = map[string]int64{}
			}
			counts[date][row.FactType] += row.Total
		}
	}
	result := app.OverviewTrends{}
	if err := s.scopedFacts(ctx, orgID, stores, "statistics_plan_fact").Where("f.stat_date>=? AND f.stat_date<? AND f.fact_type=?", from, to, "enrollment_joined").Distinct("f.testee_id").Count(&result.EnrolledTestees).Error; err != nil {
		return result, err
	}
	for date := from; date.Before(to); date = date.AddDate(0, 0, 1) {
		c := counts[date.Format("2006-01-02")]
		result.Access.EntryOpened = append(result.Access.EntryOpened, daily(date, c["entry_opened"]))
		result.Access.IntakeConfirmed = append(result.Access.IntakeConfirmed, daily(date, c["intake_confirmed"]))
		result.Access.TesteeCreated = append(result.Access.TesteeCreated, daily(date, c["testee_created"]))
		result.Access.CareRelationshipEstablished = append(result.Access.CareRelationshipEstablished, daily(date, c["care_relationship_established"]))
		result.Assessment.AnswerSheetSubmitted = append(result.Assessment.AnswerSheetSubmitted, daily(date, c["answersheet_submitted"]))
		result.Assessment.AssessmentCreated = append(result.Assessment.AssessmentCreated, daily(date, c["assessment_created"]))
		result.Assessment.ReportGenerated = append(result.Assessment.ReportGenerated, daily(date, c["report_generated"]))
		result.Assessment.AssessmentFailed = append(result.Assessment.AssessmentFailed, daily(date, c["assessment_failed"]))
		result.PlanActivity.TaskCreated = append(result.PlanActivity.TaskCreated, daily(date, c["task_created"]))
		result.PlanActivity.TaskOpened = append(result.PlanActivity.TaskOpened, daily(date, c["task_opened"]))
		result.PlanActivity.TaskCompleted = append(result.PlanActivity.TaskCompleted, daily(date, c["task_completed"]))
		result.PlanActivity.TaskExpired = append(result.PlanActivity.TaskExpired, daily(date, c["task_expired"]))
	}
	return result, nil
}
