package statisticsv2

import (
	"context"
	"fmt"

	statisticsv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type AccessDailyProjection struct{ db *gorm.DB }
type AssessmentDailyProjection struct{ db *gorm.DB }
type PlanActivityProjection struct{ db *gorm.DB }
type PlanFulfillmentProjection struct{ db *gorm.DB }
type OrganizationSnapshotProjection struct{ db *gorm.DB }

func NewProjections(db *gorm.DB) []statisticsv2.Projection {
	return []statisticsv2.Projection{
		&AccessDailyProjection{db}, &AssessmentDailyProjection{db}, &PlanActivityProjection{db},
		&PlanFulfillmentProjection{db}, &OrganizationSnapshotProjection{db},
	}
}

func projectionDB(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := mysql.TxFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

func replaceWindow(ctx context.Context, db *gorm.DB, table, dateColumn, insertSQL string, request statisticsv2.ProjectionRequest, args ...any) (int64, error) {
	handle := projectionDB(ctx, db)
	if err := handle.Exec(fmt.Sprintf("DELETE FROM %s WHERE org_id = ? AND %s >= ? AND %s < ?", table, dateColumn, dateColumn), request.OrgID, request.Window.From, request.Window.To).Error; err != nil {
		return 0, err
	}
	result := handle.Exec(insertSQL, args...)
	return result.RowsAffected, result.Error
}

func (*AccessDailyProjection) Name() string { return "access_daily" }
func (p *AccessDailyProjection) Project(ctx context.Context, r statisticsv2.ProjectionRequest) (statisticsv2.ProjectionResult, error) {
	rows, err := replaceWindow(ctx, p.db, "statistics_access_daily", "stat_date", `
		INSERT INTO statistics_access_daily (org_id,stat_date,clinician_id,entry_id,entry_opened_count,intake_confirmed_count,testee_created_count,care_relationship_established_count,care_relationship_transferred_count)
		SELECT org_id,stat_date,COALESCE(clinician_id,0),COALESCE(entry_id,0),
		SUM(fact_type='entry_opened'),SUM(fact_type='intake_confirmed'),SUM(fact_type='testee_created'),
		SUM(fact_type='care_relationship_established'),SUM(fact_type='care_relationship_transferred')
		FROM statistics_access_fact WHERE org_id=? AND stat_date>=? AND stat_date<?
		GROUP BY org_id,stat_date,COALESCE(clinician_id,0),COALESCE(entry_id,0)`, r, r.OrgID, r.Window.From, r.Window.To)
	return statisticsv2.ProjectionResult{Name: p.Name(), Rows: rows}, err
}

func (*AssessmentDailyProjection) Name() string { return "assessment_daily" }
func (p *AssessmentDailyProjection) Project(ctx context.Context, r statisticsv2.ProjectionRequest) (statisticsv2.ProjectionResult, error) {
	rows, err := replaceWindow(ctx, p.db, "statistics_assessment_daily", "stat_date", `
		INSERT INTO statistics_assessment_daily (org_id,stat_date,clinician_id,entry_id,origin_type,questionnaire_code,model_kind,model_code,answersheet_submitted_count,assessment_created_count,outcome_committed_count,assessment_failed_count,report_generated_count,report_failed_count)
		SELECT org_id,stat_date,COALESCE(clinician_id,0),COALESCE(entry_id,0),COALESCE(origin_type,''),COALESCE(questionnaire_code,''),COALESCE(model_kind,''),COALESCE(model_code,''),
		SUM(fact_type='answersheet_submitted'),SUM(fact_type='assessment_created'),SUM(fact_type='outcome_committed'),SUM(fact_type='assessment_failed'),SUM(fact_type='report_generated'),SUM(fact_type='report_failed')
		FROM statistics_assessment_fact WHERE org_id=? AND stat_date>=? AND stat_date<?
		GROUP BY org_id,stat_date,COALESCE(clinician_id,0),COALESCE(entry_id,0),COALESCE(origin_type,''),COALESCE(questionnaire_code,''),COALESCE(model_kind,''),COALESCE(model_code,'')`, r, r.OrgID, r.Window.From, r.Window.To)
	return statisticsv2.ProjectionResult{Name: p.Name(), Rows: rows}, err
}

func (*PlanActivityProjection) Name() string { return "plan_activity_daily" }
func (p *PlanActivityProjection) Project(ctx context.Context, r statisticsv2.ProjectionRequest) (statisticsv2.ProjectionResult, error) {
	rows, err := replaceWindow(ctx, p.db, "statistics_plan_activity_daily", "stat_date", `
		INSERT INTO statistics_plan_activity_daily (org_id,stat_date,plan_id,enrollment_joined_count,enrollment_closed_count,enrollment_terminated_count,task_created_count,task_opened_count,task_completed_count,task_expired_count,task_canceled_count,participant_count)
		SELECT org_id,stat_date,plan_id,SUM(fact_type='enrollment_joined'),SUM(fact_type='enrollment_closed'),SUM(fact_type='enrollment_terminated'),
		SUM(fact_type='task_created'),SUM(fact_type='task_opened'),SUM(fact_type='task_completed'),SUM(fact_type='task_expired'),SUM(fact_type='task_canceled'),COUNT(DISTINCT testee_id)
		FROM statistics_plan_fact WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY org_id,stat_date,plan_id`, r, r.OrgID, r.Window.From, r.Window.To)
	return statisticsv2.ProjectionResult{Name: p.Name(), Rows: rows}, err
}

func (*PlanFulfillmentProjection) Name() string { return "plan_fulfillment" }
func (p *PlanFulfillmentProjection) Project(ctx context.Context, r statisticsv2.ProjectionRequest) (statisticsv2.ProjectionResult, error) {
	db := projectionDB(ctx, p.db)
	if err := db.Exec("DELETE FROM statistics_plan_fulfillment_daily WHERE org_id=?", r.OrgID).Error; err != nil {
		return statisticsv2.ProjectionResult{Name: p.Name()}, err
	}
	result := db.Exec(`
		INSERT INTO statistics_plan_fulfillment_daily (org_id,cohort_date,plan_id,planned_task_count,planned_participant_count,due_task_count,completed_on_time_count,completed_overdue_count,uncompleted_overdue_count)
		WITH tasks AS (
		 SELECT c.org_id,c.plan_id,c.task_id,c.testee_id,MAX(c.planned_at) planned_at,MAX(c.due_at) due_at,
		        MAX(CASE WHEN f.fact_type='task_completed' THEN f.completed_at END) completed_at,
		        MAX(CASE WHEN f.fact_type='task_canceled' THEN 1 ELSE 0 END) canceled
		 FROM statistics_plan_fact c LEFT JOIN statistics_plan_fact f ON f.org_id=c.org_id AND f.task_id=c.task_id
		 WHERE c.org_id=? AND EXISTS (SELECT 1 FROM statistics_plan_fact created WHERE created.org_id=c.org_id AND created.task_id=c.task_id AND created.fact_type='task_created')
		 GROUP BY c.org_id,c.plan_id,c.task_id,c.testee_id
		), buckets AS (
		 SELECT org_id,DATE(planned_at) cohort_date,plan_id,COUNT(*) planned_task_count,COUNT(DISTINCT testee_id) planned_participant_count,0 due_task_count,0 completed_on_time_count,0 completed_overdue_count,0 uncompleted_overdue_count FROM tasks WHERE canceled=0 GROUP BY org_id,DATE(planned_at),plan_id
		 UNION ALL
		 SELECT org_id,DATE(due_at),plan_id,0,0,COUNT(*),SUM(completed_at IS NOT NULL AND completed_at<=due_at),SUM(completed_at>due_at),SUM(completed_at IS NULL AND due_at<?) FROM tasks WHERE due_at IS NOT NULL AND canceled=0 GROUP BY org_id,DATE(due_at),plan_id
		)
		SELECT org_id,cohort_date,plan_id,SUM(planned_task_count),SUM(planned_participant_count),SUM(due_task_count),SUM(completed_on_time_count),SUM(completed_overdue_count),SUM(uncompleted_overdue_count) FROM buckets GROUP BY org_id,cohort_date,plan_id`, r.OrgID, r.CutoffAt)
	return statisticsv2.ProjectionResult{Name: p.Name(), Rows: result.RowsAffected}, result.Error
}

func (*OrganizationSnapshotProjection) Name() string { return "organization_snapshot" }
func (p *OrganizationSnapshotProjection) Project(ctx context.Context, r statisticsv2.ProjectionRequest) (statisticsv2.ProjectionResult, error) {
	result := projectionDB(ctx, p.db).Exec(`
		INSERT INTO statistics_org_snapshot (org_id,as_of_date,snapshot_at,testee_count,clinician_count,active_clinician_count,entry_count,active_entry_count,active_enrollment_count,answersheet_submission_count,assessment_count,report_count,content_count)
		SELECT ?,?,?,
		 (SELECT COUNT(*) FROM testee WHERE org_id=? AND deleted_at IS NULL),
		 (SELECT COUNT(*) FROM clinician WHERE org_id=? AND deleted_at IS NULL),
		 (SELECT COUNT(*) FROM clinician WHERE org_id=? AND is_active=1 AND deleted_at IS NULL),
		 (SELECT COUNT(*) FROM assessment_entry WHERE org_id=? AND deleted_at IS NULL),
		 (SELECT COUNT(*) FROM assessment_entry WHERE org_id=? AND is_active=1 AND deleted_at IS NULL),
		 (SELECT COUNT(*) FROM plan_enrollment WHERE org_id=? AND status='active' AND deleted_at IS NULL),
		 (SELECT COUNT(*) FROM statistics_assessment_fact WHERE org_id=? AND fact_type='answersheet_submitted'),
		 (SELECT COUNT(*) FROM statistics_assessment_fact WHERE org_id=? AND fact_type='assessment_created'),
		 (SELECT COUNT(*) FROM statistics_assessment_fact WHERE org_id=? AND fact_type='report_generated'),
		 (SELECT COUNT(DISTINCT questionnaire_code) + COUNT(DISTINCT CASE WHEN model_code IS NOT NULL AND model_code<>'' THEN CONCAT(model_kind,'|',model_code) END) FROM statistics_assessment_fact WHERE org_id=?)
		ON DUPLICATE KEY UPDATE as_of_date=VALUES(as_of_date),snapshot_at=VALUES(snapshot_at),testee_count=VALUES(testee_count),clinician_count=VALUES(clinician_count),active_clinician_count=VALUES(active_clinician_count),entry_count=VALUES(entry_count),active_entry_count=VALUES(active_entry_count),active_enrollment_count=VALUES(active_enrollment_count),answersheet_submission_count=VALUES(answersheet_submission_count),assessment_count=VALUES(assessment_count),report_count=VALUES(report_count),content_count=VALUES(content_count)`,
		r.OrgID, r.AsOfDate, r.SnapshotAt, r.OrgID, r.OrgID, r.OrgID, r.OrgID, r.OrgID, r.OrgID, r.OrgID, r.OrgID, r.OrgID, r.OrgID)
	return statisticsv2.ProjectionResult{Name: p.Name(), Rows: result.RowsAffected}, result.Error
}
