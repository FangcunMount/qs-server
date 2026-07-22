package statistics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	domainstats "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"gorm.io/gorm"
)

type ReadStore struct {
	db      *gorm.DB
	limiter backpressure.Acquirer
}

func NewReadStore(db *gorm.DB, limiter backpressure.Acquirer) *ReadStore {
	return &ReadStore{db: db, limiter: limiter}
}

func (s *ReadStore) acquire(ctx context.Context) (context.Context, func(), error) {
	if s == nil || s.limiter == nil {
		return ctx, func() {}, nil
	}
	return s.limiter.Acquire(ctx)
}

func (s *ReadStore) LatestVisibleSnapshot(ctx context.Context, orgID int64) (*statisticsApp.Snapshot, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var row struct {
		ID              uint64
		AsOfDate        time.Time
		SnapshotAt      time.Time
		CacheGeneration int64
	}
	err = s.db.WithContext(ctx).Raw(`
		SELECT id,as_of_date,COALESCE(data_committed_at,finished_at,started_at) snapshot_at,cache_generation
		FROM statistics_sync_run
		WHERE org_id=? AND run_mode='publish'
		  AND (status='succeeded' OR (status='data_committed' AND cache_generation>0))
		ORDER BY id DESC LIMIT 1`, orgID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.AsOfDate.IsZero() {
		return nil, nil
	}
	var unsafeCount int64
	err = s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM statistics_sync_run
		WHERE org_id=? AND id>? AND (
		  (run_mode='repair' AND status='succeeded') OR
		  (run_mode='publish' AND status='data_committed' AND cache_generation=0)
		)`, orgID, row.ID).Scan(&unsafeCount).Error
	if err != nil {
		return nil, err
	}
	return &statisticsApp.Snapshot{
		AsOfDate: row.AsOfDate, SnapshotAt: row.SnapshotAt,
		VisibleRunID: row.ID, CacheGeneration: row.CacheGeneration, DatabaseReadable: unsafeCount == 0,
	}, nil
}

func (s *ReadStore) SnapshotForDate(ctx context.Context, orgID int64, asOfDate time.Time) (*statisticsApp.Snapshot, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var row struct{ AsOfDate, SnapshotAt time.Time }
	if err := s.db.WithContext(ctx).Raw(`SELECT as_of_date,snapshot_at FROM statistics_org_snapshot WHERE org_id=? AND as_of_date=? LIMIT 1`, orgID, asOfDate).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.AsOfDate.IsZero() {
		return nil, nil
	}
	return &statisticsApp.Snapshot{AsOfDate: row.AsOfDate, SnapshotAt: row.SnapshotAt, DatabaseReadable: true}, nil
}

func (s *ReadStore) Overview(ctx context.Context, orgID int64, from, to time.Time) (statisticsApp.OverviewMetrics, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return statisticsApp.OverviewMetrics{}, err
	}
	defer release()
	var value statisticsApp.OverviewMetrics
	err = s.db.WithContext(ctx).Raw(`
		SELECT o.testee_count,o.clinician_count,o.active_clinician_count,o.entry_count,o.active_entry_count,o.active_enrollment_count,
		 o.answersheet_submission_count,o.assessment_count,o.report_count,o.content_count,
		 COALESCE(a.entry_opened_count,0) entry_opened_count,COALESCE(a.intake_confirmed_count,0) intake_confirmed_count,
		 COALESCE(a.testee_created_count,0) testee_created_count,COALESCE(a.care_relationship_established_count,0) care_relationship_established_count,
		 COALESCE(a.care_relationship_transferred_count,0) care_relationship_transferred_count,
		 COALESCE(e.answersheet_submitted_count,0) window_answersheet_submitted_count,COALESCE(e.assessment_created_count,0) window_assessment_created_count,
		 COALESCE(e.outcome_committed_count,0) window_outcome_committed_count,COALESCE(e.assessment_failed_count,0) window_assessment_failed_count,
		 COALESCE(e.report_generated_count,0) window_report_generated_count,COALESCE(e.report_failed_count,0) window_report_failed_count,
		 COALESCE(p.task_created_count,0) task_created_count,COALESCE(p.task_opened_count,0) task_opened_count,
		 COALESCE(p.task_completed_count,0) task_completed_count,COALESCE(p.task_expired_count,0) task_expired_count,COALESCE(p.task_canceled_count,0) task_canceled_count,
		 COALESCE(f.planned_task_count,0) planned_task_count,COALESCE(f.due_task_count,0) due_task_count,
		 COALESCE(f.completed_on_time_count,0) completed_on_time_count,COALESCE(f.completed_overdue_count,0) completed_overdue_count,
		 COALESCE(f.uncompleted_overdue_count,0) uncompleted_overdue_count
		FROM statistics_org_snapshot o
		LEFT JOIN (SELECT org_id,SUM(entry_opened_count) entry_opened_count,SUM(intake_confirmed_count) intake_confirmed_count,SUM(testee_created_count) testee_created_count,SUM(care_relationship_established_count) care_relationship_established_count,SUM(care_relationship_transferred_count) care_relationship_transferred_count FROM statistics_access_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY org_id) a ON a.org_id=o.org_id
		LEFT JOIN (SELECT org_id,SUM(answersheet_submitted_count) answersheet_submitted_count,SUM(assessment_created_count) assessment_created_count,SUM(outcome_committed_count) outcome_committed_count,SUM(assessment_failed_count) assessment_failed_count,SUM(report_generated_count) report_generated_count,SUM(report_failed_count) report_failed_count FROM statistics_assessment_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY org_id) e ON e.org_id=o.org_id
		LEFT JOIN (SELECT org_id,SUM(task_created_count) task_created_count,SUM(task_opened_count) task_opened_count,SUM(task_completed_count) task_completed_count,SUM(task_expired_count) task_expired_count,SUM(task_canceled_count) task_canceled_count FROM statistics_plan_activity_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY org_id) p ON p.org_id=o.org_id
		LEFT JOIN (SELECT org_id,SUM(planned_task_count) planned_task_count,SUM(due_task_count) due_task_count,SUM(completed_on_time_count) completed_on_time_count,SUM(completed_overdue_count) completed_overdue_count,SUM(uncompleted_overdue_count) uncompleted_overdue_count FROM statistics_plan_fulfillment_daily WHERE org_id=? AND cohort_date>=? AND cohort_date<? GROUP BY org_id) f ON f.org_id=o.org_id
		WHERE o.org_id=?`, orgID, from, to, orgID, from, to, orgID, from, to, orgID, from, to, orgID).Scan(&value).Error
	return value, err
}

func (s *ReadStore) OverviewTrends(ctx context.Context, orgID int64, from, to time.Time) (statisticsApp.OverviewTrends, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return statisticsApp.OverviewTrends{}, err
	}
	defer release()
	type accessRow struct {
		Date                                                                     time.Time
		EntryOpened, IntakeConfirmed, TesteeCreated, CareRelationshipEstablished int64
	}
	type assessmentRow struct {
		Date                                                                       time.Time
		AnswerSheetSubmitted, AssessmentCreated, ReportGenerated, AssessmentFailed int64
	}
	type planRow struct {
		Date                                                time.Time
		TaskCreated, TaskOpened, TaskCompleted, TaskExpired int64
	}
	type fulfillmentRow struct {
		Date                                                                time.Time
		Planned, Due, CompletedOnTime, CompletedOverdue, UncompletedOverdue int64
	}
	var access []accessRow
	if err := s.db.WithContext(ctx).Raw(`SELECT stat_date date,SUM(entry_opened_count) entry_opened,SUM(intake_confirmed_count) intake_confirmed,SUM(testee_created_count) testee_created,SUM(care_relationship_established_count) care_relationship_established FROM statistics_access_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY stat_date ORDER BY stat_date`, orgID, from, to).Scan(&access).Error; err != nil {
		return statisticsApp.OverviewTrends{}, err
	}
	var assessments []assessmentRow
	if err := s.db.WithContext(ctx).Raw(`SELECT stat_date date,SUM(answersheet_submitted_count) answer_sheet_submitted,SUM(assessment_created_count) assessment_created,SUM(report_generated_count) report_generated,SUM(assessment_failed_count) assessment_failed FROM statistics_assessment_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY stat_date ORDER BY stat_date`, orgID, from, to).Scan(&assessments).Error; err != nil {
		return statisticsApp.OverviewTrends{}, err
	}
	var plans []planRow
	if err := s.db.WithContext(ctx).Raw(`SELECT stat_date date,SUM(task_created_count) task_created,SUM(task_opened_count) task_opened,SUM(task_completed_count) task_completed,SUM(task_expired_count) task_expired FROM statistics_plan_activity_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY stat_date ORDER BY stat_date`, orgID, from, to).Scan(&plans).Error; err != nil {
		return statisticsApp.OverviewTrends{}, err
	}
	var fulfillment []fulfillmentRow
	if err := s.db.WithContext(ctx).Raw(`SELECT cohort_date date,SUM(planned_task_count) planned,SUM(due_task_count) due,SUM(completed_on_time_count) completed_on_time,SUM(completed_overdue_count) completed_overdue,SUM(uncompleted_overdue_count) uncompleted_overdue FROM statistics_plan_fulfillment_daily WHERE org_id=? AND cohort_date>=? AND cohort_date<? GROUP BY cohort_date ORDER BY cohort_date`, orgID, from, to).Scan(&fulfillment).Error; err != nil {
		return statisticsApp.OverviewTrends{}, err
	}
	var enrolled int64
	if err := s.db.WithContext(ctx).Raw(`SELECT COUNT(DISTINCT testee_id) FROM statistics_plan_fact WHERE org_id=? AND fact_type='enrollment_joined' AND occurred_at>=? AND occurred_at<?`, orgID, from, to).Scan(&enrolled).Error; err != nil {
		return statisticsApp.OverviewTrends{}, err
	}

	accessByDate := make(map[string]accessRow, len(access))
	for _, row := range access {
		accessByDate[row.Date.Format("2006-01-02")] = row
	}
	assessmentByDate := make(map[string]assessmentRow, len(assessments))
	for _, row := range assessments {
		assessmentByDate[row.Date.Format("2006-01-02")] = row
	}
	planByDate := make(map[string]planRow, len(plans))
	for _, row := range plans {
		planByDate[row.Date.Format("2006-01-02")] = row
	}
	fulfillmentByDate := make(map[string]fulfillmentRow, len(fulfillment))
	for _, row := range fulfillment {
		fulfillmentByDate[row.Date.Format("2006-01-02")] = row
	}
	result := statisticsApp.OverviewTrends{EnrolledTestees: enrolled}
	for date := from; date.Before(to); date = date.AddDate(0, 0, 1) {
		key := date.Format("2006-01-02")
		a, e, p, f := accessByDate[key], assessmentByDate[key], planByDate[key], fulfillmentByDate[key]
		result.Access.EntryOpened = append(result.Access.EntryOpened, daily(date, a.EntryOpened))
		result.Access.IntakeConfirmed = append(result.Access.IntakeConfirmed, daily(date, a.IntakeConfirmed))
		result.Access.TesteeCreated = append(result.Access.TesteeCreated, daily(date, a.TesteeCreated))
		result.Access.CareRelationshipEstablished = append(result.Access.CareRelationshipEstablished, daily(date, a.CareRelationshipEstablished))
		result.Assessment.AnswerSheetSubmitted = append(result.Assessment.AnswerSheetSubmitted, daily(date, e.AnswerSheetSubmitted))
		result.Assessment.AssessmentCreated = append(result.Assessment.AssessmentCreated, daily(date, e.AssessmentCreated))
		result.Assessment.ReportGenerated = append(result.Assessment.ReportGenerated, daily(date, e.ReportGenerated))
		result.Assessment.AssessmentFailed = append(result.Assessment.AssessmentFailed, daily(date, e.AssessmentFailed))
		result.PlanActivity.TaskCreated = append(result.PlanActivity.TaskCreated, daily(date, p.TaskCreated))
		result.PlanActivity.TaskOpened = append(result.PlanActivity.TaskOpened, daily(date, p.TaskOpened))
		result.PlanActivity.TaskCompleted = append(result.PlanActivity.TaskCompleted, daily(date, p.TaskCompleted))
		result.PlanActivity.TaskExpired = append(result.PlanActivity.TaskExpired, daily(date, p.TaskExpired))
		result.PlanFulfillment.Planned = append(result.PlanFulfillment.Planned, daily(date, f.Planned))
		result.PlanFulfillment.Due = append(result.PlanFulfillment.Due, daily(date, f.Due))
		result.PlanFulfillment.Completed = append(result.PlanFulfillment.Completed, daily(date, f.CompletedOnTime+f.CompletedOverdue))
		result.PlanFulfillment.Overdue = append(result.PlanFulfillment.Overdue, daily(date, f.CompletedOverdue+f.UncompletedOverdue))
	}
	return result, nil
}

func daily(date time.Time, count int64) domainstats.DailyCount {
	return domainstats.DailyCount{Date: date, Count: count}
}

func (s *ReadStore) ListClinicians(ctx context.Context, orgID int64, clinicianID *uint64, operatorUserID *int64, from, to time.Time, page, size int) ([]statisticsApp.ClinicianItem, int64, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	where := []string{"c.org_id=?", "c.deleted_at IS NULL"}
	args := []any{orgID}
	if clinicianID != nil {
		where = append(where, "c.id=?")
		args = append(args, *clinicianID)
	}
	if operatorUserID != nil {
		where = append(where, "s.user_id=?", "s.deleted_at IS NULL")
		args = append(args, *operatorUserID)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := s.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM clinician c LEFT JOIN staff s ON s.id=c.operator_id WHERE "+whereSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	queryArgs := []any{orgID, from, to, orgID, from, to, orgID, orgID}
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, size, (page-1)*size)
	var items []statisticsApp.ClinicianItem
	err = s.db.WithContext(ctx).Raw(`SELECT c.id,c.operator_id,c.name,c.department,c.title,c.clinician_type,c.is_active,
		COALESCE(a.entry_opened_count,0) entry_opened_count,COALESCE(a.intake_confirmed_count,0) intake_confirmed_count,COALESCE(a.care_relationship_established_count,0) care_relationship_established_count,
		COALESCE(e.assessment_created_count,0) assessment_created_count,COALESCE(e.outcome_committed_count,0) outcome_committed_count,COALESCE(e.report_generated_count,0) report_generated_count,
		COALESCE(r.primary_testee_count,0) primary_testee_count,COALESCE(r.attending_testee_count,0) attending_testee_count,COALESCE(r.collaborator_testee_count,0) collaborator_testee_count,COALESCE(r.total_accessible_testees,0) total_accessible_testees,COALESCE(en.active_entry_count,0) active_entry_count
		FROM clinician c LEFT JOIN staff s ON s.id=c.operator_id
		LEFT JOIN (SELECT clinician_id,SUM(entry_opened_count) entry_opened_count,SUM(intake_confirmed_count) intake_confirmed_count,SUM(care_relationship_established_count) care_relationship_established_count FROM statistics_access_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY clinician_id) a ON a.clinician_id=c.id
		LEFT JOIN (SELECT clinician_id,SUM(assessment_created_count) assessment_created_count,SUM(outcome_committed_count) outcome_committed_count,SUM(report_generated_count) report_generated_count FROM statistics_assessment_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY clinician_id) e ON e.clinician_id=c.id
		LEFT JOIN (SELECT clinician_id,COUNT(DISTINCT CASE WHEN relation_type='primary' THEN testee_id END) primary_testee_count,COUNT(DISTINCT CASE WHEN relation_type='attending' THEN testee_id END) attending_testee_count,COUNT(DISTINCT CASE WHEN relation_type='collaborator' THEN testee_id END) collaborator_testee_count,COUNT(DISTINCT testee_id) total_accessible_testees FROM clinician_relation WHERE org_id=? AND is_active=1 AND deleted_at IS NULL GROUP BY clinician_id) r ON r.clinician_id=c.id
		LEFT JOIN (SELECT clinician_id,COUNT(*) active_entry_count FROM assessment_entry WHERE org_id=? AND is_active=1 AND deleted_at IS NULL GROUP BY clinician_id) en ON en.clinician_id=c.id
		WHERE `+whereSQL+` ORDER BY c.id LIMIT ? OFFSET ?`, queryArgs...).Scan(&items).Error
	return items, total, err
}

func (s *ReadStore) ListEntries(ctx context.Context, orgID int64, entryID, clinicianID *uint64, active *bool, from, to time.Time, page, size int) ([]statisticsApp.EntryItem, int64, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	where := []string{"en.org_id=?", "en.deleted_at IS NULL"}
	args := []any{orgID}
	if entryID != nil {
		where = append(where, "en.id=?")
		args = append(args, *entryID)
	}
	if clinicianID != nil {
		where = append(where, "en.clinician_id=?")
		args = append(args, *clinicianID)
	}
	if active != nil {
		where = append(where, "en.is_active=?")
		args = append(args, *active)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := s.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM assessment_entry en WHERE "+whereSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	queryArgs := []any{orgID, from, to, orgID, from, to}
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, size, (page-1)*size)
	var items []statisticsApp.EntryItem
	err = s.db.WithContext(ctx).Raw(`SELECT en.id,en.clinician_id,c.name clinician_name,en.token,en.target_type,en.target_code,COALESCE(en.target_version,'') target_version,en.is_active,en.expires_at,en.created_at,
		COALESCE(a.entry_opened_count,0) entry_opened_count,COALESCE(a.intake_confirmed_count,0) intake_confirmed_count,
		COALESCE(e.assessment_created_count,0) assessment_created_count,COALESCE(e.outcome_committed_count,0) outcome_committed_count,COALESCE(e.report_generated_count,0) report_generated_count
		FROM assessment_entry en LEFT JOIN clinician c ON c.id=en.clinician_id
		LEFT JOIN (SELECT entry_id,SUM(entry_opened_count) entry_opened_count,SUM(intake_confirmed_count) intake_confirmed_count FROM statistics_access_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY entry_id) a ON a.entry_id=en.id
		LEFT JOIN (SELECT entry_id,SUM(assessment_created_count) assessment_created_count,SUM(outcome_committed_count) outcome_committed_count,SUM(report_generated_count) report_generated_count FROM statistics_assessment_daily WHERE org_id=? AND stat_date>=? AND stat_date<? GROUP BY entry_id) e ON e.entry_id=en.id
		WHERE `+whereSQL+` ORDER BY en.id LIMIT ? OFFSET ?`, queryArgs...).Scan(&items).Error
	return items, total, err
}

func (s *ReadStore) CurrentClinicianID(ctx context.Context, orgID, userID int64) (uint64, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	var id uint64
	err = s.db.WithContext(ctx).Raw(`SELECT c.id FROM clinician c JOIN staff s ON s.id=c.operator_id WHERE c.org_id=? AND s.user_id=? AND c.is_active=1 AND c.deleted_at IS NULL AND s.deleted_at IS NULL LIMIT 1`, orgID, userID).Scan(&id).Error
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, errors.WithCode(code.ErrPermissionDenied, "current operator is not an active clinician")
	}
	return id, nil
}

func (s *ReadStore) CurrentClinicianTesteeSummary(ctx context.Context, orgID int64, clinicianID uint64, from, to time.Time) (statisticsApp.TesteeSummary, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return statisticsApp.TesteeSummary{}, err
	}
	defer release()
	var value statisticsApp.TesteeSummary
	err = s.db.WithContext(ctx).Raw(`SELECT
		COUNT(DISTINCT r.testee_id) total_accessible_testees,
		COUNT(DISTINCT CASE WHEN r.relation_type='primary' THEN r.testee_id END) primary_testee_count,
		COUNT(DISTINCT CASE WHEN r.relation_type='attending' THEN r.testee_id END) attending_testee_count,
		COUNT(DISTINCT CASE WHEN r.relation_type='collaborator' THEN r.testee_id END) collaborator_testee_count,
		COUNT(DISTINCT CASE WHEN t.is_key_focus=1 THEN r.testee_id END) key_focus_testee_count,
		COUNT(DISTINCT CASE WHEN f.testee_id IS NOT NULL THEN r.testee_id END) assessed_in_window_count
		FROM clinician_relation r JOIN testee t ON t.id=r.testee_id AND t.deleted_at IS NULL
		LEFT JOIN (SELECT DISTINCT testee_id FROM statistics_assessment_fact WHERE org_id=? AND fact_type='outcome_committed' AND occurred_at>=? AND occurred_at<?) f ON f.testee_id=r.testee_id
		WHERE r.org_id=? AND r.clinician_id=? AND r.is_active=1 AND r.deleted_at IS NULL`, orgID, from, to, orgID, clinicianID).Scan(&value).Error
	return value, err
}

func (s *ReadStore) ContentBatch(ctx context.Context, orgID int64, asOfDate time.Time, refs []statisticsApp.ContentRef) ([]statisticsApp.ContentItem, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	items := make([]statisticsApp.ContentItem, 0, len(refs))
	for _, ref := range refs {
		item := statisticsApp.ContentItem{Kind: ref.Kind, Code: ref.Code, HasCompletion: ref.Kind != "questionnaire"}
		if ref.Kind == "questionnaire" {
			err = s.db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(answersheet_submitted_count),0) total_submissions FROM statistics_assessment_daily WHERE org_id=? AND stat_date<=? AND questionnaire_code=?`, orgID, asOfDate, ref.Code).Scan(&item).Error
		} else {
			err = s.db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(assessment_created_count),0) total_submissions,COALESCE(SUM(outcome_committed_count),0) total_completions FROM statistics_assessment_daily WHERE org_id=? AND stat_date<=? AND model_kind=? AND model_code=?`, orgID, asOfDate, ref.Kind, ref.Code).Scan(&item).Error
			if item.TotalSubmissions > 0 {
				item.CompletionRate = float64(item.TotalCompletions) * 100 / float64(item.TotalSubmissions)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("content %s/%s: %w", ref.Kind, ref.Code, err)
		}
		items = append(items, item)
	}
	return items, nil
}
