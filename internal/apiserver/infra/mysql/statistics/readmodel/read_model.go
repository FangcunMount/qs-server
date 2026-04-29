package readmodel

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorDomainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	evaluationInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/statisticsreadmodel"
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

type readModel struct {
	db *gorm.DB
}

// NewReadModel creates the MySQL-backed statistics read model adapter.
func NewReadModel(db *gorm.DB) statisticsreadmodel.ReadModel {
	return &readModel{db: db}
}

func (m *readModel) GetOrgOverviewSnapshot(ctx context.Context, orgID int64) (domainStatistics.OrgOverviewSnapshot, error) {
	snapshot := domainStatistics.OrgOverviewSnapshot{}

	if err := m.db.WithContext(ctx).Model(&actorInfra.TesteePO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&snapshot.TesteeCount).Error; err != nil {
		return snapshot, err
	}
	if err := m.db.WithContext(ctx).Model(&actorInfra.ClinicianPO{}).
		Where("org_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, true).
		Count(&snapshot.ClinicianCount).Error; err != nil {
		return snapshot, err
	}
	if err := m.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, true).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Count(&snapshot.ActiveEntryCount).Error; err != nil {
		return snapshot, err
	}
	if err := m.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&snapshot.AssessmentCount).Error; err != nil {
		return snapshot, err
	}
	if err := m.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
		Where("org_id = ? AND status = ? AND deleted_at IS NULL", orgID, "interpreted").
		Count(&snapshot.InterpretedAssessmentCount).Error; err != nil {
		return snapshot, err
	}

	return snapshot, nil
}

func (m *readModel) GetOrgOverviewWindow(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.OrgOverviewWindow, error) {
	type behaviorRow struct {
		EntryOpenedCount       sql.NullInt64
		IntakeConfirmedCount   sql.NullInt64
		NewTestees             sql.NullInt64
		CareEstablishedCount   sql.NullInt64
		AssessmentCreatedCount sql.NullInt64
		ReportGeneratedCount   sql.NullInt64
	}

	var entryCreatedCount int64
	if err := m.db.WithContext(ctx).
		Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("created_at >= ? AND created_at < ?", from, to).
		Count(&entryCreatedCount).Error; err != nil {
		return domainStatistics.OrgOverviewWindow{}, err
	}

	var row behaviorRow
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.AnalyticsProjectionOrgDailyPO{}).
		Select(`
			COALESCE(SUM(entry_opened_count), 0) AS entry_opened_count,
			COALESCE(SUM(intake_confirmed_count), 0) AS intake_confirmed_count,
			COALESCE(SUM(testee_profile_created_count), 0) AS new_testees,
			COALESCE(SUM(care_relationship_established_count), 0) AS care_established_count,
			COALESCE(SUM(assessment_created_count), 0) AS assessment_created_count,
			COALESCE(SUM(report_generated_count), 0) AS report_generated_count
		`).
		Where("org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID,
			beginningOfDay(from),
			beginningOfDay(to),
		).
		Scan(&row).Error; err != nil {
		return domainStatistics.OrgOverviewWindow{}, err
	}

	return domainStatistics.OrgOverviewWindow{
		NewTestees:               row.NewTestees.Int64,
		EntryCreatedCount:        entryCreatedCount,
		EntryResolvedCount:       row.EntryOpenedCount.Int64,
		EntryIntakeCount:         row.IntakeConfirmedCount.Int64,
		RelationAssignedCount:    row.CareEstablishedCount.Int64,
		AssessmentCreatedCount:   row.AssessmentCreatedCount.Int64,
		AssessmentCompletedCount: row.ReportGeneratedCount.Int64,
	}, nil
}

func (m *readModel) ListOrgOverviewTrend(ctx context.Context, orgID int64, metric statisticsreadmodel.OrgOverviewMetric, from, to time.Time) []domainStatistics.DailyCount {
	field, ok := overviewTrendField(metric)
	if !ok {
		return []domainStatistics.DailyCount{}
	}

	type row struct {
		StatDate time.Time
		Count    int64
	}

	var rows []row
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.AnalyticsProjectionOrgDailyPO{}).
		Select(fmt.Sprintf("stat_date, COALESCE(%s, 0) AS count", field)).
		Where("org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID,
			beginningOfDay(from),
			beginningOfDay(to),
		).
		Order("stat_date ASC").
		Scan(&rows).Error; err != nil {
		return []domainStatistics.DailyCount{}
	}

	result := make([]domainStatistics.DailyCount, 0, len(rows))
	for _, item := range rows {
		result = append(result, domainStatistics.DailyCount{
			Date:  item.StatDate,
			Count: item.Count,
		})
	}
	return result
}

func (m *readModel) GetOrganizationOverview(ctx context.Context, orgID int64) (domainStatistics.OrganizationOverview, error) {
	overview := domainStatistics.OrganizationOverview{}

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
		Where("org_id = ? AND interpreted_at IS NOT NULL AND deleted_at IS NULL", orgID).
		Count(&overview.ReportCount).Error; err != nil {
		return overview, err
	}

	return overview, nil
}

func (m *readModel) GetAccessFunnel(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AccessFunnelWindow, error) {
	var row struct {
		EntryOpenedCount                 sql.NullInt64
		IntakeConfirmedCount             sql.NullInt64
		TesteeCreatedCount               sql.NullInt64
		CareRelationshipEstablishedCount sql.NullInt64
	}
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.AnalyticsAccessOrgDailyPO{}).
		Select(`
			COALESCE(SUM(entry_opened_count), 0) AS entry_opened_count,
			COALESCE(SUM(intake_confirmed_count), 0) AS intake_confirmed_count,
			COALESCE(SUM(testee_created_count), 0) AS testee_created_count,
			COALESCE(SUM(care_relationship_established_count), 0) AS care_relationship_established_count
		`).
		Where("org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, beginningOfDay(from), beginningOfDay(to)).
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

func (m *readModel) ListAccessFunnelTrend(ctx context.Context, orgID int64, metric statisticsreadmodel.AccessFunnelMetric, from, to time.Time) []domainStatistics.DailyCount {
	field, ok := accessFunnelTrendField(metric)
	if !ok {
		return []domainStatistics.DailyCount{}
	}
	return m.listOrgDailyTrend(ctx, &statisticsInfra.AnalyticsAccessOrgDailyPO{}, field, orgID, from, to)
}

func (m *readModel) GetAssessmentService(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.AssessmentServiceWindow, error) {
	var row struct {
		AnswerSheetSubmittedCount sql.NullInt64
		AssessmentCreatedCount    sql.NullInt64
		ReportGeneratedCount      sql.NullInt64
		AssessmentFailedCount     sql.NullInt64
	}
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.AnalyticsAssessmentServiceOrgDailyPO{}).
		Select(`
			COALESCE(SUM(answersheet_submitted_count), 0) AS answersheet_submitted_count,
			COALESCE(SUM(assessment_created_count), 0) AS assessment_created_count,
			COALESCE(SUM(report_generated_count), 0) AS report_generated_count,
			COALESCE(SUM(assessment_failed_count), 0) AS assessment_failed_count
		`).
		Where("org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, beginningOfDay(from), beginningOfDay(to)).
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

func (m *readModel) ListAssessmentServiceTrend(ctx context.Context, orgID int64, metric statisticsreadmodel.AssessmentServiceMetric, from, to time.Time) []domainStatistics.DailyCount {
	field, ok := assessmentServiceTrendField(metric)
	if !ok {
		return []domainStatistics.DailyCount{}
	}
	return m.listOrgDailyTrend(ctx, &statisticsInfra.AnalyticsAssessmentServiceOrgDailyPO{}, field, orgID, from, to)
}

func (m *readModel) GetDimensionAnalysisSummary(ctx context.Context, orgID int64) (domainStatistics.DimensionAnalysisSummary, error) {
	summary := domainStatistics.DimensionAnalysisSummary{}
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
				CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END AS content_type,
				COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS content_code
			FROM assessment
			WHERE org_id = ? AND deleted_at IS NULL
			  AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
		) content`, orgID).Scan(&contentRow).Error; err != nil {
		return summary, err
	}
	summary.ContentCount = contentRow.Count
	return summary, nil
}

func (m *readModel) GetPlanTaskOverview(ctx context.Context, orgID int64, from, to time.Time) (domainStatistics.PlanTaskWindow, error) {
	return m.getPlanTaskOverview(ctx, orgID, nil, from, to)
}

func (m *readModel) GetPlanTaskOverviewByPlan(ctx context.Context, orgID int64, planID uint64, from, to time.Time) (domainStatistics.PlanTaskWindow, error) {
	return m.getPlanTaskOverview(ctx, orgID, &planID, from, to)
}

func (m *readModel) ListPlanTaskTrend(ctx context.Context, orgID int64, planID *uint64, metric statisticsreadmodel.PlanTaskMetric, from, to time.Time) []domainStatistics.DailyCount {
	field, ok := planTaskTrendField(metric)
	if !ok {
		return []domainStatistics.DailyCount{}
	}

	type row struct {
		StatDate time.Time
		Count    int64
	}
	query := m.db.WithContext(ctx).
		Model(&statisticsInfra.AnalyticsPlanTaskDailyPO{}).
		Select(fmt.Sprintf("stat_date, COALESCE(SUM(%s), 0) AS count", field)).
		Where("org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, beginningOfDay(from), beginningOfDay(to)).
		Group("stat_date").
		Order("stat_date ASC")
	if planID != nil {
		query = query.Where("plan_id = ?", *planID)
	}
	var rows []row
	if err := query.Scan(&rows).Error; err != nil {
		return []domainStatistics.DailyCount{}
	}
	result := make([]domainStatistics.DailyCount, 0, len(rows))
	for _, item := range rows {
		result = append(result, domainStatistics.DailyCount{Date: item.StatDate, Count: item.Count})
	}
	return result
}

func (m *readModel) CountClinicianSubjects(ctx context.Context, orgID int64) (int64, error) {
	var total int64
	if err := m.db.WithContext(ctx).Model(&actorInfra.ClinicianPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (m *readModel) ListClinicianSubjects(ctx context.Context, orgID int64, page, pageSize int) ([]domainStatistics.ClinicianStatisticsSubject, error) {
	var clinicians []actorInfra.ClinicianPO
	if err := m.db.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
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

func (m *readModel) GetClinicianProjection(ctx context.Context, orgID int64, clinicianID uint64, from, to time.Time) (domainStatistics.ClinicianStatisticsWindow, domainStatistics.ClinicianStatisticsFunnel, error) {
	type behaviorRow struct {
		EntryOpenedCount       sql.NullInt64
		IntakeConfirmedCount   sql.NullInt64
		CareEstablishedCount   sql.NullInt64
		AssessmentCreatedCount sql.NullInt64
		ReportGeneratedCount   sql.NullInt64
	}

	var createdCount int64
	if err := m.db.WithContext(ctx).
		Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND clinician_id = ? AND deleted_at IS NULL", orgID, clinicianID).
		Where("created_at >= ? AND created_at < ?", from, to).
		Count(&createdCount).Error; err != nil {
		return domainStatistics.ClinicianStatisticsWindow{}, domainStatistics.ClinicianStatisticsFunnel{}, err
	}

	var row behaviorRow
	if err := m.db.WithContext(ctx).
		Model(&statisticsInfra.AnalyticsProjectionClinicianDailyPO{}).
		Select(`
			COALESCE(SUM(entry_opened_count), 0) AS entry_opened_count,
			COALESCE(SUM(intake_confirmed_count), 0) AS intake_confirmed_count,
			COALESCE(SUM(care_relationship_established_count), 0) AS care_established_count,
			COALESCE(SUM(assessment_created_count), 0) AS assessment_created_count,
			COALESCE(SUM(report_generated_count), 0) AS report_generated_count
		`).
		Where("org_id = ? AND clinician_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID,
			clinicianID,
			beginningOfDay(from),
			beginningOfDay(to),
		).
		Scan(&row).Error; err != nil {
		return domainStatistics.ClinicianStatisticsWindow{}, domainStatistics.ClinicianStatisticsFunnel{}, err
	}

	window := domainStatistics.ClinicianStatisticsWindow{
		IntakeCount:              row.IntakeConfirmedCount.Int64,
		AssignedCount:            row.CareEstablishedCount.Int64,
		CompletedAssessmentCount: row.ReportGeneratedCount.Int64,
	}
	funnel := domainStatistics.ClinicianStatisticsFunnel{
		CreatedCount:    createdCount,
		ResolvedCount:   row.EntryOpenedCount.Int64,
		IntakeCount:     row.IntakeConfirmedCount.Int64,
		AssignedCount:   row.CareEstablishedCount.Int64,
		AssessmentCount: row.AssessmentCreatedCount.Int64,
	}

	return window, funnel, nil
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
	query := m.assessmentEntryQuery(ctx, orgID, clinicianID, activeOnly)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (m *readModel) ListAssessmentEntryMetas(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, page, pageSize int) ([]domainStatistics.AssessmentEntryStatisticsMeta, error) {
	query := m.assessmentEntryQuery(ctx, orgID, clinicianID, activeOnly)

	var entries []actorInfra.AssessmentEntryPO
	if err := query.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&entries).Error; err != nil {
		return nil, err
	}

	items := make([]domainStatistics.AssessmentEntryStatisticsMeta, 0, len(entries))
	for _, entry := range entries {
		metaItem, err := m.enrichAssessmentEntryMeta(ctx, orgID, entry)
		if err != nil {
			return nil, err
		}
		items = append(items, metaItem)
	}
	return items, nil
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

	metaItem, err := m.enrichAssessmentEntryMeta(ctx, orgID, entry)
	if err != nil {
		return nil, err
	}
	return &metaItem, nil
}

func (m *readModel) GetAssessmentEntryCounts(ctx context.Context, orgID int64, entryID uint64, from, to *time.Time) (domainStatistics.AssessmentEntryStatisticsCounts, error) {
	type projectionRow struct {
		EntryOpenedCount       sql.NullInt64
		IntakeConfirmedCount   sql.NullInt64
		CareEstablishedCount   sql.NullInt64
		AssessmentCreatedCount sql.NullInt64
	}

	query := m.db.WithContext(ctx).
		Model(&statisticsInfra.AnalyticsProjectionEntryDailyPO{}).
		Select(`
			COALESCE(SUM(entry_opened_count), 0) AS entry_opened_count,
			COALESCE(SUM(intake_confirmed_count), 0) AS intake_confirmed_count,
			COALESCE(SUM(care_relationship_established_count), 0) AS care_established_count,
			COALESCE(SUM(assessment_created_count), 0) AS assessment_created_count
		`).
		Where("org_id = ? AND entry_id = ? AND deleted_at IS NULL",
			orgID,
			entryID,
		)
	if from != nil && to != nil {
		query = query.Where("stat_date >= ? AND stat_date < ?", beginningOfDay(*from), beginningOfDay(*to))
	}

	var row projectionRow
	if err := query.Scan(&row).Error; err != nil {
		return domainStatistics.AssessmentEntryStatisticsCounts{}, err
	}

	return domainStatistics.AssessmentEntryStatisticsCounts{
		ResolveCount:    row.EntryOpenedCount.Int64,
		IntakeCount:     row.IntakeConfirmedCount.Int64,
		AssignedCount:   row.CareEstablishedCount.Int64,
		AssessmentCount: row.AssessmentCreatedCount.Int64,
	}, nil
}

func (m *readModel) GetAssessmentEntryLastEventTime(ctx context.Context, orgID int64, entryID uint64, eventName domainStatistics.BehaviorEventName) (*time.Time, error) {
	return queryNullableMaxTime(
		m.db.WithContext(ctx).
			Model(&statisticsInfra.BehaviorFootprintPO{}).
			Select("MAX(occurred_at)").
			Where("org_id = ? AND entry_id = ? AND event_name = ? AND deleted_at IS NULL",
				orgID,
				entryID,
				string(eventName),
			),
	)
}

func (m *readModel) GetQuestionnaireBatchTotals(ctx context.Context, orgID int64, codes []string) ([]statisticsreadmodel.QuestionnaireBatchTotal, error) {
	type row struct {
		Code             string
		TotalSubmissions int64
		TotalCompletions int64
	}

	var rows []row
	if err := m.db.WithContext(ctx).
		Table("assessment").
		Select("COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS code, COUNT(*) AS total_submissions, SUM(CASE WHEN status = 'interpreted' THEN 1 ELSE 0 END) AS total_completions").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) IN ?", codes).
		Group("COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code)").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]statisticsreadmodel.QuestionnaireBatchTotal, 0, len(rows))
	for _, rowItem := range rows {
		items = append(items, statisticsreadmodel.QuestionnaireBatchTotal{
			Code:             rowItem.Code,
			TotalSubmissions: rowItem.TotalSubmissions,
			TotalCompletions: rowItem.TotalCompletions,
		})
	}
	return items, nil
}

func (m *readModel) assessmentEntryQuery(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool) *gorm.DB {
	query := m.db.WithContext(ctx).
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

func (m *readModel) enrichAssessmentEntryMeta(ctx context.Context, orgID int64, entry actorInfra.AssessmentEntryPO) (domainStatistics.AssessmentEntryStatisticsMeta, error) {
	metaItem := assessmentEntryMetaFromPO(entry)

	var clinician actorInfra.ClinicianPO
	if err := m.db.WithContext(ctx).
		Select("id, name").
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, entry.ClinicianID.Uint64()).
		First(&clinician).Error; err == nil {
		metaItem.ClinicianName = clinician.Name
	}

	return metaItem, nil
}

func (m *readModel) listOrgDailyTrend(ctx context.Context, model interface{}, field string, orgID int64, from, to time.Time) []domainStatistics.DailyCount {
	type row struct {
		StatDate time.Time
		Count    int64
	}
	var rows []row
	if err := m.db.WithContext(ctx).
		Model(model).
		Select(fmt.Sprintf("stat_date, COALESCE(%s, 0) AS count", field)).
		Where("org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, beginningOfDay(from), beginningOfDay(to)).
		Order("stat_date ASC").
		Scan(&rows).Error; err != nil {
		return []domainStatistics.DailyCount{}
	}

	result := make([]domainStatistics.DailyCount, 0, len(rows))
	for _, item := range rows {
		result = append(result, domainStatistics.DailyCount{Date: item.StatDate, Count: item.Count})
	}
	return result
}

func (m *readModel) getPlanTaskOverview(ctx context.Context, orgID int64, planID *uint64, from, to time.Time) (domainStatistics.PlanTaskWindow, error) {
	var row struct {
		TaskCreatedCount   sql.NullInt64
		TaskOpenedCount    sql.NullInt64
		TaskCompletedCount sql.NullInt64
		TaskExpiredCount   sql.NullInt64
	}
	query := m.db.WithContext(ctx).
		Model(&statisticsInfra.AnalyticsPlanTaskDailyPO{}).
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
		return domainStatistics.PlanTaskWindow{}, err
	}

	enrolledTestees, err := m.countPlanTaskDistinctTestees(ctx, orgID, planID, "created_at", "", from, to)
	if err != nil {
		return domainStatistics.PlanTaskWindow{}, err
	}
	activeTestees, err := m.countPlanTaskDistinctTestees(ctx, orgID, planID, "completed_at", "completed", from, to)
	if err != nil {
		return domainStatistics.PlanTaskWindow{}, err
	}

	return domainStatistics.PlanTaskWindow{
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
	query := m.db.WithContext(ctx).
		Table("assessment_task t").
		Joins("JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL").
		Select("COUNT(DISTINCT t.testee_id) AS count").
		Where("t.org_id = ? AND t.deleted_at IS NULL", orgID).
		Where(fmt.Sprintf("t.%s >= ? AND t.%s < ?", timeField, timeField), from, to)
	if planID != nil {
		query = query.Where("t.plan_id = ?", *planID)
	}
	if status != "" {
		query = query.Where("t.status = ?", status)
	}
	if err := scanCountQuery(query, &count); err != nil {
		return 0, err
	}
	return count, nil
}

func overviewTrendField(metric statisticsreadmodel.OrgOverviewMetric) (string, bool) {
	switch metric {
	case statisticsreadmodel.OrgOverviewMetricAssessmentCreated:
		return "assessment_created_count", true
	case statisticsreadmodel.OrgOverviewMetricIntakeConfirmed:
		return "intake_confirmed_count", true
	case statisticsreadmodel.OrgOverviewMetricRelationAssigned:
		return "care_relationship_established_count", true
	default:
		return "", false
	}
}

func accessFunnelTrendField(metric statisticsreadmodel.AccessFunnelMetric) (string, bool) {
	switch metric {
	case statisticsreadmodel.AccessFunnelMetricEntryOpened:
		return "entry_opened_count", true
	case statisticsreadmodel.AccessFunnelMetricIntakeConfirmed:
		return "intake_confirmed_count", true
	case statisticsreadmodel.AccessFunnelMetricTesteeCreated:
		return "testee_created_count", true
	case statisticsreadmodel.AccessFunnelMetricCareRelationshipEstablished:
		return "care_relationship_established_count", true
	default:
		return "", false
	}
}

func assessmentServiceTrendField(metric statisticsreadmodel.AssessmentServiceMetric) (string, bool) {
	switch metric {
	case statisticsreadmodel.AssessmentServiceMetricAnswerSheetSubmitted:
		return "answersheet_submitted_count", true
	case statisticsreadmodel.AssessmentServiceMetricAssessmentCreated:
		return "assessment_created_count", true
	case statisticsreadmodel.AssessmentServiceMetricReportGenerated:
		return "report_generated_count", true
	case statisticsreadmodel.AssessmentServiceMetricAssessmentFailed:
		return "assessment_failed_count", true
	default:
		return "", false
	}
}

func planTaskTrendField(metric statisticsreadmodel.PlanTaskMetric) (string, bool) {
	switch metric {
	case statisticsreadmodel.PlanTaskMetricCreated:
		return "task_created_count", true
	case statisticsreadmodel.PlanTaskMetricOpened:
		return "task_opened_count", true
	case statisticsreadmodel.PlanTaskMetricCompleted:
		return "task_completed_count", true
	case statisticsreadmodel.PlanTaskMetricExpired:
		return "task_expired_count", true
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

func queryNullableMaxTime(query *gorm.DB) (*time.Time, error) {
	var value sql.NullTime
	if err := query.Scan(&value).Error; err != nil {
		return nil, err
	}
	if !value.Valid {
		return nil, nil
	}

	t := value.Time
	return &t, nil
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
