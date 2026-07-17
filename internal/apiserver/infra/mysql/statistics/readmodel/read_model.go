package readmodel

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	actorDomainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	evaluationInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

var accessGrantRelationTypes = []string{
	string(actorDomainRelation.RelationTypeAssigned),
	string(actorDomainRelation.RelationTypePrimary),
	string(actorDomainRelation.RelationTypeAttending),
	string(actorDomainRelation.RelationTypeCollaborator),
}

const (
	planTaskStatusCompleted = "completed"
	planTaskStatusExpired   = "expired"
	planTaskStatusCanceled  = "canceled"
)

type readModel struct {
	db *gorm.DB
}

// NewReadModel creates the MySQL-backed statistics read model adapter.
func NewReadModel(db *gorm.DB) *readModel {
	return &readModel{db: db}
}

func (m *readModel) GetOrganizationOverview(ctx context.Context, orgID int64) (domainStatistics.OrganizationOverview, error) {
	overview := domainStatistics.OrganizationOverview{}

	var snapshot statisticsInfra.StatisticsOrgSnapshotPO
	if err := m.db.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		First(&snapshot).Error; err == nil {
		overview = domainStatistics.OrganizationOverview{
			TesteeCount:      snapshot.TesteeCount,
			ClinicianCount:   snapshot.ClinicianCount,
			ActiveEntryCount: snapshot.ActiveEntryCount,
			AssessmentCount:  snapshot.AssessmentCount,
			ReportCount:      snapshot.ReportCount,
			ContentCount:     snapshot.DimensionContentCount,
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return overview, err
	} else {
		if err := m.db.WithContext(ctx).Model(&actorInfra.TesteePO{}).
			Where("org_id = ? AND deleted_at IS NULL", orgID).
			Count(&overview.TesteeCount).Error; err != nil {
			return overview, err
		}
		if err := m.db.WithContext(ctx).Model(&actorInfra.ClinicianPO{}).
			Where("org_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, true).
			Count(&overview.ClinicianCount).Error; err != nil {
			return overview, err
		}
		if err := m.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).
			Where("org_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, true).
			Where("expires_at IS NULL OR expires_at > ?", time.Now()).
			Count(&overview.ActiveEntryCount).Error; err != nil {
			return overview, err
		}
		if err := m.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
			Where("org_id = ? AND deleted_at IS NULL", orgID).
			Count(&overview.AssessmentCount).Error; err != nil {
			return overview, err
		}
		if err := m.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
			Where("org_id = ? AND status = ? AND evaluated_at IS NOT NULL AND deleted_at IS NULL", orgID, "evaluated").
			Count(&overview.ReportCount).Error; err != nil {
			return overview, err
		}
		contentCount, err := m.countOrganizationContent(ctx, orgID)
		if err != nil {
			return overview, err
		}
		overview.ContentCount = contentCount
	}

	if err := m.fillOrganizationAnswerSheetSubmissions(ctx, orgID, &overview); err != nil {
		return overview, err
	}
	return overview, nil
}

func (m *readModel) fillOrganizationAnswerSheetSubmissions(ctx context.Context, orgID int64, overview *domainStatistics.OrganizationOverview) error {
	todayStart := beginningOfDay(time.Now())
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	var projectionRows int64
	if err := m.db.WithContext(ctx).Model(&statisticsInfra.StatisticsContentDailyPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&projectionRows).Error; err != nil {
		return err
	}
	if projectionRows > 0 {
		var row struct {
			Total int64
			Today int64
		}
		if err := m.db.WithContext(ctx).Model(&statisticsInfra.StatisticsContentDailyPO{}).
			Select("COALESCE(SUM(answersheet_submitted_count), 0) AS total, COALESCE(SUM(CASE WHEN stat_date >= ? AND stat_date < ? THEN answersheet_submitted_count ELSE 0 END), 0) AS today", todayStart, tomorrowStart).
			Where("org_id = ? AND deleted_at IS NULL", orgID).
			Scan(&row).Error; err != nil {
			return err
		}
		overview.AnswerSheetSubmissionCount = row.Total
		overview.TodayAnswerSheetSubmissionCount = row.Today
		return nil
	}

	if err := m.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
		Where("org_id = ? AND submitted_at IS NOT NULL AND deleted_at IS NULL", orgID).
		Count(&overview.AnswerSheetSubmissionCount).Error; err != nil {
		return err
	}
	return m.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
		Where("org_id = ? AND submitted_at >= ? AND submitted_at < ? AND deleted_at IS NULL", orgID, todayStart, tomorrowStart).
		Count(&overview.TodayAnswerSheetSubmissionCount).Error
}

func (m *readModel) countOrganizationContent(ctx context.Context, orgID int64) (int64, error) {
	var row struct {
		Count int64 `gorm:"column:count"`
	}
	err := m.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) AS count
		FROM (
			SELECT DISTINCT
				CASE WHEN evaluation_model_kind = 'scale' THEN 'scale' ELSE 'questionnaire' END AS content_type,
				COALESCE(NULLIF(CASE WHEN evaluation_model_kind = 'scale' THEN evaluation_model_code END, ''), questionnaire_code) AS content_code
			FROM assessment
			WHERE org_id = ? AND deleted_at IS NULL
			  AND COALESCE(NULLIF(CASE WHEN evaluation_model_kind = 'scale' THEN evaluation_model_code END, ''), questionnaire_code) <> ''
		) content`, orgID).Scan(&row).Error
	return row.Count, err
}

func (m *readModel) GetAccessFunnel(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AccessFunnelWindow, error) {
	var row struct {
		EntryOpenedCount                 sql.NullInt64
		IntakeConfirmedCount             sql.NullInt64
		TesteeCreatedCount               sql.NullInt64
		CareRelationshipEstablishedCount sql.NullInt64
	}
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.StatisticsJourneyDailyPO{}).
		Select(`
			COALESCE(SUM(access_entry_opened_count), 0) AS entry_opened_count,
			COALESCE(SUM(access_intake_confirmed_count), 0) AS intake_confirmed_count,
			COALESCE(SUM(access_testee_created_count), 0) AS testee_created_count,
			COALESCE(SUM(access_care_relationship_established_count), 0) AS care_relationship_established_count
		`).
		Where("org_id = ? AND subject_type = ? AND subject_id = 0 AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID, statisticsInfra.StatisticsJourneySubjectOrg, beginningOfDay(from), beginningOfDay(to)).
		Scan(&row).Error; err != nil {
		return domainStatistics.AccessFunnelWindow{}, err
	}
	return domainStatistics.AccessFunnelWindow{
		EntryOpenedCount:                 row.EntryOpenedCount.Int64,
		IntakeConfirmedCount:             row.IntakeConfirmedCount.Int64,
		TesteeCreatedCount:               row.TesteeCreatedCount.Int64,
		CareRelationshipEstablishedCount: row.CareRelationshipEstablishedCount.Int64,
	}, nil
}

func (m *readModel) GetAccessFunnelTrend(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AccessFunnelTrend, error) {
	type row struct {
		StatDate                         time.Time
		EntryOpenedCount                 int64
		IntakeConfirmedCount             int64
		TesteeCreatedCount               int64
		CareRelationshipEstablishedCount int64
	}
	var rows []row
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.StatisticsJourneyDailyPO{}).
		Select(`
			stat_date,
			COALESCE(access_entry_opened_count, 0) AS entry_opened_count,
			COALESCE(access_intake_confirmed_count, 0) AS intake_confirmed_count,
			COALESCE(access_testee_created_count, 0) AS testee_created_count,
			COALESCE(access_care_relationship_established_count, 0) AS care_relationship_established_count
		`).
		Where("org_id = ? AND subject_type = ? AND subject_id = 0 AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID, statisticsInfra.StatisticsJourneySubjectOrg, beginningOfDay(from), beginningOfDay(to)).
		Order("stat_date ASC").
		Scan(&rows).Error; err != nil {
		return domainStatistics.AccessFunnelTrend{}, err
	}
	trend := domainStatistics.AccessFunnelTrend{}
	for _, item := range rows {
		trend.EntryOpened = append(trend.EntryOpened, domainStatistics.DailyCount{Date: item.StatDate, Count: item.EntryOpenedCount})
		trend.IntakeConfirmed = append(trend.IntakeConfirmed, domainStatistics.DailyCount{Date: item.StatDate, Count: item.IntakeConfirmedCount})
		trend.TesteeCreated = append(trend.TesteeCreated, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TesteeCreatedCount})
		trend.CareRelationshipEstablished = append(trend.CareRelationshipEstablished, domainStatistics.DailyCount{Date: item.StatDate, Count: item.CareRelationshipEstablishedCount})
	}
	return trend, nil
}

const assessmentServiceAnswerSheetSubmittedScanAlias = "answer_sheet_submitted_count"

func (m *readModel) GetAssessmentService(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AssessmentServiceWindow, error) {
	var row struct {
		AnswerSheetSubmittedCount sql.NullInt64
		AssessmentCreatedCount    sql.NullInt64
		ReportGeneratedCount      sql.NullInt64
		AssessmentFailedCount     sql.NullInt64
	}
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.StatisticsJourneyDailyPO{}).
		Select(`
			COALESCE(SUM(service_answersheet_submitted_count), 0) AS `+assessmentServiceAnswerSheetSubmittedScanAlias+`,
			COALESCE(SUM(service_assessment_created_count), 0) AS assessment_created_count,
			COALESCE(SUM(service_report_generated_count), 0) AS report_generated_count,
			COALESCE(SUM(service_assessment_failed_count), 0) AS assessment_failed_count
		`).
		Where("org_id = ? AND subject_type = ? AND subject_id = 0 AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID, statisticsInfra.StatisticsJourneySubjectOrg, beginningOfDay(from), beginningOfDay(to)).
		Scan(&row).Error; err != nil {
		return domainStatistics.AssessmentServiceWindow{}, err
	}
	return domainStatistics.AssessmentServiceWindow{
		AnswerSheetSubmittedCount: row.AnswerSheetSubmittedCount.Int64,
		AssessmentCreatedCount:    row.AssessmentCreatedCount.Int64,
		ReportGeneratedCount:      row.ReportGeneratedCount.Int64,
		AssessmentFailedCount:     row.AssessmentFailedCount.Int64,
	}, nil
}

func (m *readModel) GetAssessmentServiceTrend(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AssessmentServiceTrend, error) {
	type row struct {
		StatDate                  time.Time
		AnswerSheetSubmittedCount int64
		AssessmentCreatedCount    int64
		ReportGeneratedCount      int64
		AssessmentFailedCount     int64
	}
	var rows []row
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.StatisticsJourneyDailyPO{}).
		Select(`
			stat_date,
			COALESCE(service_answersheet_submitted_count, 0) AS `+assessmentServiceAnswerSheetSubmittedScanAlias+`,
			COALESCE(service_assessment_created_count, 0) AS assessment_created_count,
			COALESCE(service_report_generated_count, 0) AS report_generated_count,
			COALESCE(service_assessment_failed_count, 0) AS assessment_failed_count
		`).
		Where("org_id = ? AND subject_type = ? AND subject_id = 0 AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID, statisticsInfra.StatisticsJourneySubjectOrg, beginningOfDay(from), beginningOfDay(to)).
		Order("stat_date ASC").
		Scan(&rows).Error; err != nil {
		return domainStatistics.AssessmentServiceTrend{}, err
	}
	trend := domainStatistics.AssessmentServiceTrend{}
	for _, item := range rows {
		trend.AnswerSheetSubmitted = append(trend.AnswerSheetSubmitted, domainStatistics.DailyCount{Date: item.StatDate, Count: item.AnswerSheetSubmittedCount})
		trend.AssessmentCreated = append(trend.AssessmentCreated, domainStatistics.DailyCount{Date: item.StatDate, Count: item.AssessmentCreatedCount})
		trend.ReportGenerated = append(trend.ReportGenerated, domainStatistics.DailyCount{Date: item.StatDate, Count: item.ReportGeneratedCount})
		trend.AssessmentFailed = append(trend.AssessmentFailed, domainStatistics.DailyCount{Date: item.StatDate, Count: item.AssessmentFailedCount})
	}
	return trend, nil
}

func (m *readModel) GetDimensionAnalysisSummary(ctx context.Context, orgID int64) (domainStatistics.DimensionAnalysisSummary, error) {
	summary := domainStatistics.DimensionAnalysisSummary{}
	var snapshot statisticsInfra.StatisticsOrgSnapshotPO
	if err := m.db.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		First(&snapshot).Error; err == nil {
		return domainStatistics.DimensionAnalysisSummary{
			ClinicianCount: snapshot.DimensionClinicianCount,
			EntryCount:     snapshot.DimensionEntryCount,
			ContentCount:   snapshot.DimensionContentCount,
		}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return summary, err
	}

	if err := m.db.WithContext(ctx).Model(&actorInfra.ClinicianPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&summary.ClinicianCount).Error; err != nil {
		return summary, err
	}
	if err := m.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&summary.EntryCount).Error; err != nil {
		return summary, err
	}
	var contentRow struct {
		Count int64 `gorm:"column:count"`
	}
	if err := m.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) AS count
		FROM (
			SELECT DISTINCT
				CASE WHEN evaluation_model_kind = 'scale' THEN 'scale' ELSE 'questionnaire' END AS content_type,
				COALESCE(NULLIF(CASE WHEN evaluation_model_kind = 'scale' THEN evaluation_model_code END, ''), questionnaire_code) AS content_code
			FROM assessment
			WHERE org_id = ? AND deleted_at IS NULL
			  AND COALESCE(NULLIF(CASE WHEN evaluation_model_kind = 'scale' THEN evaluation_model_code END, ''), questionnaire_code) <> ''
		) content`, orgID).Scan(&contentRow).Error; err != nil {
		return summary, err
	}
	summary.ContentCount = contentRow.Count
	return summary, nil
}

func (m *readModel) GetPlanTaskOverview(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.PlanTaskActivityWindow, error) {
	return m.getPlanTaskOverview(ctx, orgID, nil, from, to)
}

func (m *readModel) GetPlanTaskTrend(ctx context.Context, orgID int64, planID *uint64, from, to time.Time) (domainStatistics.PlanTaskActivityTrend, error) {
	type row struct {
		StatDate           time.Time
		TaskCreatedCount   int64
		TaskOpenedCount    int64
		TaskCompletedCount int64
		TaskExpiredCount   int64
	}
	query := buildPlanTaskTrendQuery(m.db.WithContext(ctx), orgID, planID, from, to)
	var rows []row
	if err := query.Scan(&rows).Error; err != nil {
		return domainStatistics.PlanTaskActivityTrend{}, err
	}
	trend := domainStatistics.PlanTaskActivityTrend{}
	for _, item := range rows {
		trend.TaskCreated = append(trend.TaskCreated, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TaskCreatedCount})
		trend.TaskOpened = append(trend.TaskOpened, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TaskOpenedCount})
		trend.TaskCompleted = append(trend.TaskCompleted, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TaskCompletedCount})
		trend.TaskExpired = append(trend.TaskExpired, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TaskExpiredCount})
	}
	return trend, nil
}

func (m *readModel) GetPlanTaskFulfillment(ctx context.Context, orgID int64, planID *uint64, from, to time.Time) (domainStatistics.PlanTaskFulfillmentWindow, error) {
	var row struct {
		PlannedTaskCount     sql.NullInt64
		DueTaskCount         sql.NullInt64
		CompletedTaskCount   sql.NullInt64
		OnTimeCompletedCount sql.NullInt64
		OverdueTaskCount     sql.NullInt64
	}
	if err := buildPlanTaskFulfillmentWindowQuery(m.db.WithContext(ctx), orgID, planID, from, to, time.Now()).
		Scan(&row).Error; err != nil {
		return domainStatistics.PlanTaskFulfillmentWindow{}, err
	}
	return planTaskFulfillmentWindowFromRow(
		row.PlannedTaskCount.Int64,
		row.DueTaskCount.Int64,
		row.CompletedTaskCount.Int64,
		row.OnTimeCompletedCount.Int64,
		row.OverdueTaskCount.Int64,
	), nil
}

func (m *readModel) GetPlanTaskFulfillmentTrend(ctx context.Context, orgID int64, planID *uint64, from, to time.Time) (domainStatistics.PlanTaskFulfillmentTrend, error) {
	type row struct {
		StatDate           time.Time
		PlannedTaskCount   int64
		DueTaskCount       int64
		CompletedTaskCount int64
		OverdueTaskCount   int64
	}
	var rows []row
	if err := buildPlanTaskFulfillmentTrendQuery(m.db.WithContext(ctx), orgID, planID, from, to, time.Now()).
		Scan(&rows).Error; err != nil {
		return domainStatistics.PlanTaskFulfillmentTrend{}, err
	}
	trend := domainStatistics.PlanTaskFulfillmentTrend{}
	for _, item := range rows {
		trend.Planned = append(trend.Planned, domainStatistics.DailyCount{Date: item.StatDate, Count: item.PlannedTaskCount})
		trend.Due = append(trend.Due, domainStatistics.DailyCount{Date: item.StatDate, Count: item.DueTaskCount})
		trend.Completed = append(trend.Completed, domainStatistics.DailyCount{Date: item.StatDate, Count: item.CompletedTaskCount})
		trend.Overdue = append(trend.Overdue, domainStatistics.DailyCount{Date: item.StatDate, Count: item.OverdueTaskCount})
	}
	return trend, nil
}

func (m *readModel) CountClinicianSubjects(ctx context.Context, orgID int64) (int64, error) {
	var total int64
	if err := buildClinicianSubjectQuery(m.db.WithContext(ctx), orgID).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (m *readModel) ListClinicianSubjects(ctx context.Context, orgID int64, page, pageSize int) ([]domainStatistics.ClinicianStatisticsSubject, error) {
	var clinicians []actorInfra.ClinicianPO
	if err := buildClinicianSubjectQuery(m.db.WithContext(ctx), orgID).
		Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&clinicians).Error; err != nil {
		return nil, err
	}

	items := make([]domainStatistics.ClinicianStatisticsSubject, 0, len(clinicians))
	for _, clinician := range clinicians {
		items = append(items, clinicianSubjectFromPO(clinician))
	}
	return items, nil
}

func (m *readModel) GetClinicianSubject(ctx context.Context, orgID int64, clinicianID uint64) (*domainStatistics.ClinicianStatisticsSubject, error) {
	var clinician actorInfra.ClinicianPO
	if err := m.db.WithContext(ctx).
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, clinicianID).
		First(&clinician).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "clinician not found")
		}
		return nil, err
	}

	subject := clinicianSubjectFromPO(clinician)
	return &subject, nil
}

func (m *readModel) GetCurrentClinicianSubject(ctx context.Context, orgID int64, operatorUserID int64) (*domainStatistics.ClinicianStatisticsSubject, error) {
	var operator actorInfra.OperatorPO
	if err := m.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, operatorUserID, true).
		First(&operator).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "operator is not active in current organization")
		}
		return nil, err
	}

	var clinician actorInfra.ClinicianPO
	if err := m.db.WithContext(ctx).
		Where("org_id = ? AND operator_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, operator.ID.Uint64(), true).
		First(&clinician).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "current operator is not bound to an active clinician")
		}
		return nil, err
	}

	subject := clinicianSubjectFromPO(clinician)
	return &subject, nil
}

func (m *readModel) GetClinicianStatisticsDetails(ctx context.Context, orgID int64, clinicianIDs []uint64, from, to time.Time) (map[uint64]statisticsApp.ClinicianStatisticsDetail, error) {
	details := make(map[uint64]statisticsApp.ClinicianStatisticsDetail, len(clinicianIDs))
	if len(clinicianIDs) == 0 {
		return details, nil
	}

	type relationRow struct {
		ClinicianID       uint64
		PrimaryCount      int64
		AttendingCount    int64
		CollaboratorCount int64
		AccessibleCount   int64
	}
	var relationRows []relationRow
	if err := m.db.WithContext(ctx).
		Table("clinician_relation").
		Select(`clinician_id,
			COUNT(DISTINCT CASE WHEN relation_type = ? THEN testee_id END) AS primary_count,
			COUNT(DISTINCT CASE WHEN relation_type = ? THEN testee_id END) AS attending_count,
			COUNT(DISTINCT CASE WHEN relation_type = ? THEN testee_id END) AS collaborator_count,
			COUNT(DISTINCT CASE WHEN relation_type IN ? THEN testee_id END) AS accessible_count`,
			string(actorDomainRelation.RelationTypePrimary),
			string(actorDomainRelation.RelationTypeAttending),
			string(actorDomainRelation.RelationTypeCollaborator),
			accessGrantRelationTypes).
		Where("org_id = ? AND clinician_id IN ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianIDs, true).
		Group("clinician_id").
		Scan(&relationRows).Error; err != nil {
		return nil, err
	}
	for _, row := range relationRows {
		detail := details[row.ClinicianID]
		detail.Snapshot.PrimaryTesteeCount = row.PrimaryCount
		detail.Snapshot.AttendingTesteeCount = row.AttendingCount
		detail.Snapshot.CollaboratorTesteeCount = row.CollaboratorCount
		detail.Snapshot.TotalAccessibleTestees = row.AccessibleCount
		details[row.ClinicianID] = detail
	}

	type countRow struct {
		ClinicianID uint64
		Count       int64
	}
	var activeEntryRows []countRow
	if err := m.db.WithContext(ctx).
		Model(&actorInfra.AssessmentEntryPO{}).
		Select("clinician_id, COUNT(*) AS count").
		Where("org_id = ? AND clinician_id IN ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianIDs, true).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Group("clinician_id").
		Scan(&activeEntryRows).Error; err != nil {
		return nil, err
	}
	for _, row := range activeEntryRows {
		detail := details[row.ClinicianID]
		detail.Snapshot.ActiveEntryCount = row.Count
		details[row.ClinicianID] = detail
	}

	var createdRows []countRow
	if err := m.db.WithContext(ctx).
		Model(&actorInfra.AssessmentEntryPO{}).
		Select("clinician_id, COUNT(*) AS count").
		Where("org_id = ? AND clinician_id IN ? AND deleted_at IS NULL", orgID, clinicianIDs).
		Where("created_at >= ? AND created_at < ?", from, to).
		Group("clinician_id").
		Scan(&createdRows).Error; err != nil {
		return nil, err
	}
	for _, row := range createdRows {
		detail := details[row.ClinicianID]
		detail.Funnel.CreatedCount = row.Count
		details[row.ClinicianID] = detail
	}

	type journeyRow struct {
		ClinicianID            uint64
		EntryOpenedCount       int64
		IntakeConfirmedCount   int64
		CareEstablishedCount   int64
		AssessmentCreatedCount int64
		ReportGeneratedCount   int64
	}
	var journeyRows []journeyRow
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.StatisticsJourneyDailyPO{}).
		Select(`subject_id AS clinician_id,
			COALESCE(SUM(entry_opened_count), 0) AS entry_opened_count,
			COALESCE(SUM(intake_confirmed_count), 0) AS intake_confirmed_count,
			COALESCE(SUM(care_relationship_established_count), 0) AS care_established_count,
			COALESCE(SUM(assessment_created_count), 0) AS assessment_created_count,
			COALESCE(SUM(report_generated_count), 0) AS report_generated_count`).
		Where("org_id = ? AND subject_type = ? AND subject_id IN ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID, statisticsInfra.StatisticsJourneySubjectClinician, clinicianIDs, beginningOfDay(from), beginningOfDay(to)).
		Group("subject_id").
		Scan(&journeyRows).Error; err != nil {
		return nil, err
	}
	for _, row := range journeyRows {
		detail := details[row.ClinicianID]
		detail.Window = domainStatistics.ClinicianStatisticsWindow{
			IntakeCount: row.IntakeConfirmedCount, AssignedCount: row.CareEstablishedCount, CompletedAssessmentCount: row.ReportGeneratedCount,
		}
		detail.Funnel.ResolvedCount = row.EntryOpenedCount
		detail.Funnel.IntakeCount = row.IntakeConfirmedCount
		detail.Funnel.AssignedCount = row.CareEstablishedCount
		detail.Funnel.AssessmentCount = row.AssessmentCreatedCount
		details[row.ClinicianID] = detail
	}
	return details, nil
}

func (m *readModel) GetClinicianSnapshot(ctx context.Context, orgID int64, clinicianID uint64) (domainStatistics.ClinicianStatisticsSnapshot, error) {
	snapshot := domainStatistics.ClinicianStatisticsSnapshot{}

	countByType := func(relationType string) (int64, error) {
		var count int64
		err := scanCountQuery(m.db.WithContext(ctx).
			Table("clinician_relation").
			Select("COUNT(DISTINCT testee_id) AS count").
			Where("org_id = ? AND clinician_id = ? AND is_active = ? AND relation_type = ? AND deleted_at IS NULL", orgID, clinicianID, true, relationType),
			&count)
		return count, err
	}

	var err error
	if snapshot.PrimaryTesteeCount, err = countByType(string(actorDomainRelation.RelationTypePrimary)); err != nil {
		return snapshot, err
	}
	if snapshot.AttendingTesteeCount, err = countByType(string(actorDomainRelation.RelationTypeAttending)); err != nil {
		return snapshot, err
	}
	if snapshot.CollaboratorTesteeCount, err = countByType(string(actorDomainRelation.RelationTypeCollaborator)); err != nil {
		return snapshot, err
	}
	if err := scanCountQuery(m.db.WithContext(ctx).
		Table("clinician_relation").
		Select("COUNT(DISTINCT testee_id) AS count").
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL AND relation_type IN ?", orgID, clinicianID, true, accessGrantRelationTypes),
		&snapshot.TotalAccessibleTestees); err != nil {
		return snapshot, err
	}
	if err := m.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Count(&snapshot.ActiveEntryCount).Error; err != nil {
		return snapshot, err
	}

	return snapshot, nil
}

func (m *readModel) GetClinicianTesteeSummaryCounts(ctx context.Context, orgID int64, clinicianID uint64, from, to time.Time) (int64, int64, error) {
	var keyFocusCount int64
	if err := scanCountQuery(m.db.WithContext(ctx).
		Table("testee t").
		Select("COUNT(DISTINCT t.id) AS count").
		Joins("JOIN clinician_relation r ON r.testee_id = t.id AND r.org_id = t.org_id").
		Where("t.org_id = ? AND t.is_key_focus = ? AND t.deleted_at IS NULL", orgID, true).
		Where("r.clinician_id = ? AND r.is_active = ? AND r.deleted_at IS NULL AND r.relation_type IN ?", clinicianID, true, accessGrantRelationTypes),
		&keyFocusCount); err != nil {
		return 0, 0, err
	}

	var assessedInWindowCount int64
	if err := scanCountQuery(m.db.WithContext(ctx).
		Table("assessment a").
		Select("COUNT(DISTINCT a.testee_id) AS count").
		Joins("JOIN clinician_relation r ON r.testee_id = a.testee_id AND r.org_id = a.org_id").
		Where("a.org_id = ? AND a.deleted_at IS NULL", orgID).
		Where("r.clinician_id = ? AND r.is_active = ? AND r.deleted_at IS NULL AND r.relation_type IN ?", clinicianID, true, accessGrantRelationTypes).
		Where("a.created_at >= ? AND a.created_at < ?", from, to),
		&assessedInWindowCount); err != nil {
		return 0, 0, err
	}

	return keyFocusCount, assessedInWindowCount, nil
}

func (m *readModel) CountAssessmentEntries(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool) (int64, error) {
	query := buildAssessmentEntryMetaQuery(m.db.WithContext(ctx), orgID, clinicianID, activeOnly)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (m *readModel) ListAssessmentEntryMetas(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, page, pageSize int) ([]domainStatistics.AssessmentEntryStatisticsMeta, error) {
	query := buildAssessmentEntryMetaQuery(m.db.WithContext(ctx), orgID, clinicianID, activeOnly)

	var entries []actorInfra.AssessmentEntryPO
	if err := query.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&entries).Error; err != nil {
		return nil, err
	}

	return m.assessmentEntryMetasFromPO(ctx, orgID, entries)
}

func (m *readModel) GetAssessmentEntryMeta(ctx context.Context, orgID int64, entryID uint64) (*domainStatistics.AssessmentEntryStatisticsMeta, error) {
	var entry actorInfra.AssessmentEntryPO
	if err := m.db.WithContext(ctx).
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, entryID).
		First(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "assessment entry not found")
		}
		return nil, err
	}

	items, err := m.assessmentEntryMetasFromPO(ctx, orgID, []actorInfra.AssessmentEntryPO{entry})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (m *readModel) GetAssessmentEntryStatisticsDetails(ctx context.Context, orgID int64, entryIDs []uint64, from, to time.Time) (map[uint64]statisticsApp.AssessmentEntryStatisticsDetail, error) {
	details := make(map[uint64]statisticsApp.AssessmentEntryStatisticsDetail, len(entryIDs))
	if len(entryIDs) == 0 {
		return details, nil
	}
	type journeyRow struct {
		EntryID                 uint64
		SnapshotResolvedCount   int64
		SnapshotIntakeCount     int64
		SnapshotAssignedCount   int64
		SnapshotAssessmentCount int64
		WindowResolvedCount     int64
		WindowIntakeCount       int64
		WindowAssignedCount     int64
		WindowAssessmentCount   int64
	}
	var journeyRows []journeyRow
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.StatisticsJourneyDailyPO{}).
		Select(`subject_id AS entry_id,
			COALESCE(SUM(entry_opened_count), 0) AS snapshot_resolved_count,
			COALESCE(SUM(intake_confirmed_count), 0) AS snapshot_intake_count,
			COALESCE(SUM(care_relationship_established_count), 0) AS snapshot_assigned_count,
			COALESCE(SUM(assessment_created_count), 0) AS snapshot_assessment_count,
			COALESCE(SUM(CASE WHEN stat_date >= ? AND stat_date < ? THEN entry_opened_count ELSE 0 END), 0) AS window_resolved_count,
			COALESCE(SUM(CASE WHEN stat_date >= ? AND stat_date < ? THEN intake_confirmed_count ELSE 0 END), 0) AS window_intake_count,
			COALESCE(SUM(CASE WHEN stat_date >= ? AND stat_date < ? THEN care_relationship_established_count ELSE 0 END), 0) AS window_assigned_count,
			COALESCE(SUM(CASE WHEN stat_date >= ? AND stat_date < ? THEN assessment_created_count ELSE 0 END), 0) AS window_assessment_count`,
			beginningOfDay(from), beginningOfDay(to), beginningOfDay(from), beginningOfDay(to),
			beginningOfDay(from), beginningOfDay(to), beginningOfDay(from), beginningOfDay(to)).
		Where("org_id = ? AND subject_type = ? AND subject_id IN ? AND deleted_at IS NULL", orgID, statisticsInfra.StatisticsJourneySubjectEntry, entryIDs).
		Group("subject_id").
		Scan(&journeyRows).Error; err != nil {
		return nil, err
	}
	for _, row := range journeyRows {
		details[row.EntryID] = statisticsApp.AssessmentEntryStatisticsDetail{
			Snapshot: domainStatistics.AssessmentEntryStatisticsCounts{ResolveCount: row.SnapshotResolvedCount, IntakeCount: row.SnapshotIntakeCount, AssignedCount: row.SnapshotAssignedCount, AssessmentCount: row.SnapshotAssessmentCount},
			Window:   domainStatistics.AssessmentEntryStatisticsCounts{ResolveCount: row.WindowResolvedCount, IntakeCount: row.WindowIntakeCount, AssignedCount: row.WindowAssignedCount, AssessmentCount: row.WindowAssessmentCount},
		}
	}

	type eventRow struct {
		EntryID        uint64
		LastResolvedAt sql.NullTime
		LastIntakeAt   sql.NullTime
	}
	var eventRows []eventRow
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.BehaviorFootprintPO{}).
		Select(`entry_id,
			MAX(CASE WHEN event_name = ? THEN occurred_at END) AS last_resolved_at,
			MAX(CASE WHEN event_name = ? THEN occurred_at END) AS last_intake_at`,
			string(domainStatistics.BehaviorEventEntryOpened), string(domainStatistics.BehaviorEventIntakeConfirmed)).
		Where("org_id = ? AND entry_id IN ? AND deleted_at IS NULL", orgID, entryIDs).
		Group("entry_id").
		Scan(&eventRows).Error; err != nil {
		return nil, err
	}
	for _, row := range eventRows {
		detail := details[row.EntryID]
		if row.LastResolvedAt.Valid {
			value := row.LastResolvedAt.Time
			detail.LastResolvedAt = &value
		}
		if row.LastIntakeAt.Valid {
			value := row.LastIntakeAt.Time
			detail.LastIntakeAt = &value
		}
		details[row.EntryID] = detail
	}
	return details, nil
}

func (m *readModel) GetContentBatchTotals(ctx context.Context, orgID int64, refs []statisticsApp.ContentReference) ([]statisticsApp.ContentBatchTotal, error) {
	if len(refs) == 0 {
		return []statisticsApp.ContentBatchTotal{}, nil
	}

	rows, err := loadProjectedContentBatchTotals(m.db.WithContext(ctx), orgID, refs)
	if err != nil {
		return nil, err
	}
	found := make(map[statisticsApp.ContentReference]struct{}, len(rows))
	items := make([]statisticsApp.ContentBatchTotal, 0, len(refs))
	for _, rowItem := range rows {
		ref := statisticsApp.ContentReference{Type: rowItem.Type, Code: rowItem.Code}
		found[ref] = struct{}{}
		items = append(items, rowItem)
	}

	missing := make([]statisticsApp.ContentReference, 0, len(refs))
	for _, ref := range refs {
		if _, ok := found[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	if len(missing) == 0 {
		return items, nil
	}

	realtimeRows, err := loadRealtimeContentBatchTotals(m.db.WithContext(ctx), orgID, missing)
	if err != nil {
		return nil, err
	}
	return append(items, realtimeRows...), nil
}

func buildClinicianSubjectQuery(db *gorm.DB, orgID int64) *gorm.DB {
	return db.Model(&actorInfra.ClinicianPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID)
}

func buildAssessmentEntryMetaQuery(db *gorm.DB, orgID int64, clinicianID *uint64, activeOnly *bool) *gorm.DB {
	query := db.
		Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID)
	if clinicianID != nil {
		query = query.Where("clinician_id = ?", *clinicianID)
	}
	if activeOnly != nil {
		query = query.Where("is_active = ?", *activeOnly)
	}
	return query
}

func loadProjectedContentBatchTotals(db *gorm.DB, orgID int64, refs []statisticsApp.ContentReference) ([]statisticsApp.ContentBatchTotal, error) {
	var rows []statisticsApp.ContentBatchTotal
	err := buildProjectedContentBatchTotalsQuery(db, orgID, refs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	requested := make(map[statisticsApp.ContentReference]struct{}, len(refs))
	for _, ref := range refs {
		requested[ref] = struct{}{}
	}
	filtered := rows[:0]
	for _, row := range rows {
		if _, ok := requested[statisticsApp.ContentReference{Type: row.Type, Code: row.Code}]; ok {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func buildProjectedContentBatchTotalsQuery(db *gorm.DB, orgID int64, refs []statisticsApp.ContentReference) *gorm.DB {
	types, codes := contentReferenceDimensions(refs)
	return db.
		Model(&statisticsInfra.StatisticsContentDailyPO{}).
		Select("content_type AS type, content_code AS code, COALESCE(SUM(submission_count), 0) AS total_submissions, COALESCE(SUM(completion_count), 0) AS total_completions").
		Where("org_id = ? AND content_type IN ? AND content_code IN ? AND deleted_at IS NULL", orgID, types, codes).
		Group("content_type, content_code")
}

const (
	assessmentContentTypeExpression = "CASE WHEN evaluation_model_kind = 'scale' THEN 'scale' ELSE 'questionnaire' END"
	assessmentContentCodeExpression = "COALESCE(NULLIF(CASE WHEN evaluation_model_kind = 'scale' THEN evaluation_model_code END, ''), questionnaire_code)"
)

func loadRealtimeContentBatchTotals(db *gorm.DB, orgID int64, refs []statisticsApp.ContentReference) ([]statisticsApp.ContentBatchTotal, error) {
	var rows []statisticsApp.ContentBatchTotal
	err := buildRealtimeContentBatchTotalsQuery(db, orgID, refs).Scan(&rows).Error
	return rows, err
}

func buildRealtimeContentBatchTotalsQuery(db *gorm.DB, orgID int64, refs []statisticsApp.ContentReference) *gorm.DB {
	predicates := make([]string, 0, len(refs))
	args := make([]interface{}, 0, len(refs)*2)
	for _, ref := range refs {
		predicates = append(predicates, "("+assessmentContentTypeExpression+" = ? AND "+assessmentContentCodeExpression+" = ?)")
		args = append(args, ref.Type, ref.Code)
	}
	return db.Table("assessment").
		Select(assessmentContentTypeExpression+" AS type, "+assessmentContentCodeExpression+" AS code, COUNT(*) AS total_submissions, COALESCE(SUM(CASE WHEN status = 'evaluated' THEN 1 ELSE 0 END), 0) AS total_completions").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where(strings.Join(predicates, " OR "), args...).
		Group(assessmentContentTypeExpression + ", " + assessmentContentCodeExpression)
}

func contentReferenceDimensions(refs []statisticsApp.ContentReference) ([]string, []string) {
	types := make([]string, 0, len(refs))
	codes := make([]string, 0, len(refs))
	seenTypes := make(map[string]struct{}, len(refs))
	seenCodes := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, ok := seenTypes[ref.Type]; !ok {
			seenTypes[ref.Type] = struct{}{}
			types = append(types, ref.Type)
		}
		if _, ok := seenCodes[ref.Code]; !ok {
			seenCodes[ref.Code] = struct{}{}
			codes = append(codes, ref.Code)
		}
	}
	return types, codes
}

func buildPlanTaskTrendQuery(db *gorm.DB, orgID int64, planID *uint64, from, to time.Time) *gorm.DB {
	query := db.
		Model(&statisticsInfra.StatisticsPlanDailyPO{}).
		Select(`
			stat_date,
			COALESCE(SUM(task_created_count), 0) AS task_created_count,
			COALESCE(SUM(task_opened_count), 0) AS task_opened_count,
			COALESCE(SUM(task_completed_count), 0) AS task_completed_count,
			COALESCE(SUM(task_expired_count), 0) AS task_expired_count
		`).
		Where("org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, beginningOfDay(from), beginningOfDay(to)).
		Group("stat_date").
		Order("stat_date ASC")
	if planID != nil {
		query = query.Where("plan_id = ?", *planID)
	}
	return query
}

func buildPlanTaskFulfillmentWindowQuery(db *gorm.DB, orgID int64, planID *uint64, from, to, now time.Time) *gorm.DB {
	planClause := ""
	plannedArgs := []interface{}{orgID, planTaskStatusCanceled, from, to}
	dueArgs := []interface{}{
		planTaskStatusCompleted,
		planTaskStatusCompleted,
		planTaskStatusExpired, planTaskStatusCompleted, now,
		orgID, planTaskStatusCanceled, from, to,
	}
	if planID != nil {
		planClause = " AND t.plan_id = ?"
		plannedArgs = append(plannedArgs, *planID)
		dueArgs = append(dueArgs, *planID)
	}

	sql := fmt.Sprintf(`
		SELECT
			planned.planned_task_count,
			due.due_task_count,
			due.completed_task_count,
			due.on_time_completed_count,
			due.overdue_task_count
		FROM (
			SELECT COUNT(*) AS planned_task_count
			FROM assessment_task t FORCE INDEX (idx_task_org_deleted_planned_status)
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.status <> ? AND t.planned_at >= ? AND t.planned_at < ?%s
		) planned
		CROSS JOIN (
			SELECT
				COUNT(*) AS due_task_count,
				COALESCE(SUM(CASE WHEN t.status = ? THEN 1 ELSE 0 END), 0) AS completed_task_count,
				COALESCE(SUM(CASE WHEN t.status = ? AND t.completed_at IS NOT NULL AND t.completed_at <= t.expire_at THEN 1 ELSE 0 END), 0) AS on_time_completed_count,
				COALESCE(SUM(CASE WHEN t.status = ? OR (t.completed_at IS NOT NULL AND t.completed_at > t.expire_at) OR (t.status <> ? AND t.expire_at < ?) THEN 1 ELSE 0 END), 0) AS overdue_task_count
			FROM assessment_task t FORCE INDEX (idx_task_org_deleted_expire_status)
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.status <> ? AND t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ?%s
		) due`, planClause, planClause)

	args := make([]interface{}, 0, len(plannedArgs)+len(dueArgs))
	args = append(args, plannedArgs...)
	args = append(args, dueArgs...)
	return db.Raw(sql, args...)
}

func buildPlanTaskFulfillmentTrendQuery(db *gorm.DB, orgID int64, planID *uint64, from, to, now time.Time) *gorm.DB {
	planClause := ""
	plannedArgs := []interface{}{orgID, planTaskStatusCanceled, from, to}
	dueArgs := []interface{}{planTaskStatusCompleted, planTaskStatusExpired, planTaskStatusCompleted, now, orgID, planTaskStatusCanceled, from, to}
	if planID != nil {
		planClause = " AND t.plan_id = ?"
		plannedArgs = append(plannedArgs, *planID)
		dueArgs = append(dueArgs, *planID)
	}

	sql := fmt.Sprintf(`
		SELECT
			raw.stat_date,
			COALESCE(SUM(raw.planned_task_count), 0) AS planned_task_count,
			COALESCE(SUM(raw.due_task_count), 0) AS due_task_count,
			COALESCE(SUM(raw.completed_task_count), 0) AS completed_task_count,
			COALESCE(SUM(raw.overdue_task_count), 0) AS overdue_task_count
		FROM (
			SELECT
				DATE(t.planned_at) AS stat_date,
				COUNT(*) AS planned_task_count,
				0 AS due_task_count,
				0 AS completed_task_count,
				0 AS overdue_task_count
			FROM assessment_task t FORCE INDEX (idx_task_org_deleted_planned_status)
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.status <> ? AND t.planned_at >= ? AND t.planned_at < ?%s
			GROUP BY DATE(t.planned_at)
			UNION ALL
			SELECT
				DATE(t.expire_at) AS stat_date,
				0 AS planned_task_count,
				COUNT(*) AS due_task_count,
				SUM(CASE WHEN t.status = ? THEN 1 ELSE 0 END) AS completed_task_count,
				SUM(CASE WHEN t.status = ? OR (t.completed_at IS NOT NULL AND t.completed_at > t.expire_at) OR (t.status <> ? AND t.expire_at < ?) THEN 1 ELSE 0 END) AS overdue_task_count
			FROM assessment_task t FORCE INDEX (idx_task_org_deleted_expire_status)
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.status <> ? AND t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ?%s
			GROUP BY DATE(t.expire_at)
		) raw
		GROUP BY raw.stat_date
		ORDER BY raw.stat_date ASC`, planClause, planClause)

	args := make([]interface{}, 0, len(plannedArgs)+len(dueArgs))
	args = append(args, plannedArgs...)
	args = append(args, dueArgs...)
	return db.Raw(sql, args...)
}

func planTaskFulfillmentWindowFromRow(planned, due, completed, onTimeCompleted, overdue int64) domainStatistics.PlanTaskFulfillmentWindow {
	return domainStatistics.PlanTaskFulfillmentWindow{
		PlannedTaskCount:     planned,
		DueTaskCount:         due,
		CompletedTaskCount:   completed,
		OnTimeCompletedCount: onTimeCompleted,
		OverdueTaskCount:     overdue,
		CompletionRate:       domainStatistics.CompletionRate(due, completed),
		OnTimeCompletionRate: domainStatistics.CompletionRate(due, onTimeCompleted),
	}
}

func (m *readModel) assessmentEntryMetasFromPO(ctx context.Context, orgID int64, entries []actorInfra.AssessmentEntryPO) ([]domainStatistics.AssessmentEntryStatisticsMeta, error) {
	clinicianIDs := make([]uint64, 0, len(entries))
	seen := make(map[uint64]struct{}, len(entries))
	for _, entry := range entries {
		id := entry.ClinicianID.Uint64()
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			clinicianIDs = append(clinicianIDs, id)
		}
	}
	clinicianNames := make(map[uint64]string, len(clinicianIDs))
	if len(clinicianIDs) > 0 {
		var clinicians []actorInfra.ClinicianPO
		if err := m.db.WithContext(ctx).
			Select("id, name").
			Where("org_id = ? AND id IN ? AND deleted_at IS NULL", orgID, clinicianIDs).
			Find(&clinicians).Error; err != nil {
			return nil, err
		}
		for _, clinician := range clinicians {
			clinicianNames[clinician.ID.Uint64()] = clinician.Name
		}
	}
	items := make([]domainStatistics.AssessmentEntryStatisticsMeta, 0, len(entries))
	for _, entry := range entries {
		item := assessmentEntryMetaFromPO(entry)
		item.ClinicianName = clinicianNames[entry.ClinicianID.Uint64()]
		items = append(items, item)
	}
	return items, nil
}

func (m *readModel) getPlanTaskOverview(ctx context.Context, orgID int64, planID *uint64, from, to time.Time) (domainStatistics.PlanTaskActivityWindow, error) {
	var row struct {
		TaskCreatedCount   sql.NullInt64
		TaskOpenedCount    sql.NullInt64
		TaskCompletedCount sql.NullInt64
		TaskExpiredCount   sql.NullInt64
	}
	query := m.db.WithContext(ctx).
		Model(&statisticsInfra.StatisticsPlanDailyPO{}).
		Select(`
			COALESCE(SUM(task_created_count), 0) AS task_created_count,
			COALESCE(SUM(task_opened_count), 0) AS task_opened_count,
			COALESCE(SUM(task_completed_count), 0) AS task_completed_count,
			COALESCE(SUM(task_expired_count), 0) AS task_expired_count
		`).
		Where("org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, beginningOfDay(from), beginningOfDay(to))
	if planID != nil {
		query = query.Where("plan_id = ?", *planID)
	}
	if err := query.Scan(&row).Error; err != nil {
		return domainStatistics.PlanTaskActivityWindow{}, err
	}

	enrolledTestees, err := m.countPlanTaskDistinctTestees(ctx, orgID, planID, "created_at", "", from, to)
	if err != nil {
		return domainStatistics.PlanTaskActivityWindow{}, err
	}
	activeTestees, err := m.countPlanTaskDistinctTestees(ctx, orgID, planID, "completed_at", "completed", from, to)
	if err != nil {
		return domainStatistics.PlanTaskActivityWindow{}, err
	}

	return domainStatistics.PlanTaskActivityWindow{
		TaskCreatedCount:   row.TaskCreatedCount.Int64,
		TaskOpenedCount:    row.TaskOpenedCount.Int64,
		TaskCompletedCount: row.TaskCompletedCount.Int64,
		TaskExpiredCount:   row.TaskExpiredCount.Int64,
		EnrolledTestees:    enrolledTestees,
		ActiveTestees:      activeTestees,
	}, nil
}

func (m *readModel) countPlanTaskDistinctTestees(ctx context.Context, orgID int64, planID *uint64, timeField, status string, from, to time.Time) (int64, error) {
	var count int64
	query, err := buildPlanTaskDistinctTesteeCountQuery(m.db.WithContext(ctx), orgID, planID, timeField, status, from, to)
	if err != nil {
		return 0, err
	}
	if err := scanCountQuery(query, &count); err != nil {
		return 0, err
	}
	return count, nil
}

func buildPlanTaskDistinctTesteeCountQuery(db *gorm.DB, orgID int64, planID *uint64, timeField, status string, from, to time.Time) (*gorm.DB, error) {
	indexName, ok := planTaskDistinctTesteeIndex(timeField)
	if !ok {
		return nil, fmt.Errorf("unsupported plan task distinct time field: %s", timeField)
	}

	planClause := ""
	statusClause := ""
	args := []interface{}{orgID, from, to}
	if planID != nil {
		planClause = " AND t.plan_id = ?"
		args = append(args, *planID)
	}
	if status != "" {
		statusClause = " AND t.status = ?"
		args = append(args, status)
	}
	args = append(args, orgID)

	sql := fmt.Sprintf(`
		SELECT COUNT(DISTINCT scoped.testee_id) AS count
		FROM (
			SELECT t.plan_id, t.testee_id
			FROM assessment_task t FORCE INDEX (%s)
			WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.%s >= ? AND t.%s < ?%s%s
			GROUP BY t.plan_id, t.testee_id
		) scoped
		JOIN assessment_plan p ON p.org_id = ? AND p.id = scoped.plan_id AND p.deleted_at IS NULL`,
		indexName, timeField, timeField, planClause, statusClause)
	return db.Raw(sql, args...), nil
}

func planTaskDistinctTesteeIndex(timeField string) (string, bool) {
	switch timeField {
	case "created_at":
		return "idx_task_org_deleted_created", true
	case "completed_at":
		return "idx_task_org_deleted_completed_status", true
	default:
		return "", false
	}
}

func clinicianSubjectFromPO(item actorInfra.ClinicianPO) domainStatistics.ClinicianStatisticsSubject {
	return domainStatistics.ClinicianStatisticsSubject{
		ID:            item.ID,
		OperatorID:    ptrMetaIDFromUint64(item.OperatorID),
		Name:          item.Name,
		Department:    item.Department,
		Title:         item.Title,
		ClinicianType: item.ClinicianType,
		IsActive:      item.IsActive,
	}
}

func assessmentEntryMetaFromPO(item actorInfra.AssessmentEntryPO) domainStatistics.AssessmentEntryStatisticsMeta {
	return domainStatistics.AssessmentEntryStatisticsMeta{
		ID:            item.ID,
		OrgID:         item.OrgID,
		ClinicianID:   item.ClinicianID,
		Token:         item.Token,
		TargetType:    item.TargetType,
		TargetCode:    item.TargetCode,
		TargetVersion: derefString(item.TargetVersion),
		IsActive:      item.IsActive,
		CreatedAt:     item.CreatedAt,
		ExpiresAt:     item.ExpiresAt,
	}
}

func beginningOfDay(v time.Time) time.Time {
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, v.Location())
}

func scanCountQuery(query *gorm.DB, dest *int64) error {
	var row struct {
		Count int64 `gorm:"column:count"`
	}
	if err := query.Scan(&row).Error; err != nil {
		return err
	}
	*dest = row.Count
	return nil
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ptrMetaIDFromUint64(v *uint64) *meta.ID {
	if v == nil {
		return nil
	}
	id := meta.FromUint64(*v)
	return &id
}
