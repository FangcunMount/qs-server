package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"time"
)

type scopedFulfillmentDay struct {
	CohortDate                                                          string
	Planned, Due, CompletedOnTime, CompletedOverdue, UncompletedOverdue int64
}

// scopedFulfillment retains the published schedule/legacy fulfillment contract.
// Scope selection precedes revision resolution and both planned/due buckets.
func (s *ReadStore) scopedFulfillment(ctx context.Context, orgID int64, stores authz.StoreRange, from, to, cutoff time.Time) ([]scopedFulfillmentDay, error) {
	facts := s.scopedFacts(ctx, orgID, stores, "statistics_plan_fact").Select("f.*")
	var rows []scopedFulfillmentDay
	err := s.db.WithContext(ctx).Raw(scopedFulfillmentSQL, facts, orgID, orgID, cutoff, from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&rows).Error
	return rows, err
}

const scopedFulfillmentSQL = `WITH scoped_plan_facts AS (?), schedule_ranked AS (
		 SELECT org_id,plan_id,testee_id,task_id,schedule_revision,schedule_planned_at,schedule_due_at,
		        ROW_NUMBER() OVER (PARTITION BY org_id,task_id ORDER BY schedule_revision DESC,id DESC) schedule_rank
		 FROM scoped_plan_facts
		 WHERE org_id=? AND fact_type='task_schedule_defined'
		), latest_schedule AS (
		 SELECT org_id,plan_id,testee_id,task_id,schedule_revision,schedule_planned_at,schedule_due_at
		 FROM schedule_ranked WHERE schedule_rank=1
		), latest_terminal AS (
		 SELECT terminal.org_id,terminal.task_id,terminal.task_status,terminal.completed_at
		 FROM scoped_plan_facts terminal
		 JOIN latest_schedule schedule ON schedule.org_id=terminal.org_id AND schedule.task_id=terminal.task_id AND schedule.schedule_revision=terminal.schedule_revision
		 WHERE terminal.fact_type='task_schedule_terminal'
		), schedule_tasks AS (
		 SELECT schedule.org_id,schedule.plan_id,schedule.task_id,schedule.testee_id,
		        schedule.schedule_planned_at planned_at,schedule.schedule_due_at due_at,
		        CASE WHEN terminal.task_status='completed' THEN terminal.completed_at END completed_at,
		        CASE WHEN terminal.task_status='canceled' THEN 1 ELSE 0 END canceled
		 FROM latest_schedule schedule
		 LEFT JOIN latest_terminal terminal ON terminal.org_id=schedule.org_id AND terminal.task_id=schedule.task_id
		), legacy_tasks AS (
		 SELECT created.org_id,created.plan_id,created.task_id,created.testee_id,MAX(created.planned_at) planned_at,
		        COALESCE(MAX(CASE WHEN legacy.fact_type='task_due_defined' THEN legacy.due_at END),MAX(CASE WHEN legacy.fact_type<>'task_due_defined' THEN legacy.due_at END)) due_at,
		        MAX(CASE WHEN legacy.fact_type='task_completed' THEN legacy.completed_at END) completed_at,
		        MAX(CASE WHEN legacy.fact_type='task_canceled' THEN 1 ELSE 0 END) canceled
		 FROM scoped_plan_facts created
		 LEFT JOIN scoped_plan_facts legacy ON legacy.org_id=created.org_id AND legacy.task_id=created.task_id
		  AND legacy.fact_type IN ('task_created','task_opened','task_completed','task_expired','task_canceled','task_due_defined')
		 LEFT JOIN latest_schedule schedule ON schedule.org_id=created.org_id AND schedule.task_id=created.task_id
		 WHERE created.org_id=? AND created.fact_type='task_created' AND schedule.task_id IS NULL
		 GROUP BY created.org_id,created.plan_id,created.task_id,created.testee_id
		), tasks AS (
		 SELECT * FROM schedule_tasks
		 UNION ALL
		 SELECT * FROM legacy_tasks
		), buckets AS (
		 SELECT org_id,DATE(planned_at) cohort_date,plan_id,COUNT(*) planned_task_count,COUNT(DISTINCT testee_id) planned_participant_count,0 due_task_count,0 completed_on_time_count,0 completed_overdue_count,0 uncompleted_overdue_count FROM tasks WHERE canceled=0 GROUP BY org_id,DATE(planned_at),plan_id
		 UNION ALL
		 SELECT org_id,DATE(due_at),plan_id,0,0,COUNT(*),SUM(completed_at IS NOT NULL AND completed_at<=due_at),SUM(CASE WHEN completed_at>due_at THEN 1 ELSE 0 END),SUM(completed_at IS NULL AND due_at<?) FROM tasks WHERE due_at IS NOT NULL AND canceled=0 GROUP BY org_id,DATE(due_at),plan_id
		)
 SELECT cohort_date, SUM(planned_task_count) planned, SUM(due_task_count) due,
 SUM(completed_on_time_count) completed_on_time,SUM(completed_overdue_count) completed_overdue,
 SUM(uncompleted_overdue_count) uncompleted_overdue FROM buckets
 WHERE cohort_date>=? AND cohort_date<? GROUP BY cohort_date ORDER BY cohort_date`
