package statistics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	actorDomainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	surveyAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
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

type readService struct {
	db              *gorm.DB
	answerSheetRepo surveyAnswerSheet.Repository
}

// NewReadService 创建统一统计读服务。
func NewReadService(db *gorm.DB, answerSheetRepo surveyAnswerSheet.Repository) ReadService {
	return &readService{db: db, answerSheetRepo: answerSheetRepo}
}

func (s *readService) GetOverview(ctx context.Context, orgID int64, filter QueryFilter) (*domainStatistics.StatisticsOverview, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	snapshot := domainStatistics.OrgOverviewSnapshot{}
	window := domainStatistics.OrgOverviewWindow{}

	if err := s.db.WithContext(ctx).Model(&actorInfra.TesteePO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&snapshot.TesteeCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&actorInfra.ClinicianPO{}).
		Where("org_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, true).
		Count(&snapshot.ClinicianCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, true).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Count(&snapshot.ActiveEntryCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&snapshot.AssessmentCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
		Where("org_id = ? AND status = ? AND deleted_at IS NULL", orgID, "interpreted").
		Count(&snapshot.InterpretedAssessmentCount).Error; err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(&actorInfra.TesteePO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("created_at >= ? AND created_at < ?", timeRange.From, timeRange.To).
		Count(&window.NewTestees).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("created_at >= ? AND created_at < ?", timeRange.From, timeRange.To).
		Count(&window.EntryCreatedCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&statisticsInfra.AssessmentEntryResolveLogPO{}).
		Where("org_id = ?", orgID).
		Where("resolved_at >= ? AND resolved_at < ?", timeRange.From, timeRange.To).
		Count(&window.EntryResolvedCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&actorInfra.ClinicianRelationPO{}).
		Where("org_id = ? AND relation_type = ? AND source_type = ? AND deleted_at IS NULL", orgID, string(actorDomainRelation.RelationTypeCreator), string(actorDomainRelation.SourceTypeAssessmentEntry)).
		Where("bound_at >= ? AND bound_at < ?", timeRange.From, timeRange.To).
		Count(&window.EntryIntakeCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&actorInfra.ClinicianRelationPO{}).
		Where("org_id = ? AND relation_type IN ? AND deleted_at IS NULL", orgID, accessGrantRelationTypes).
		Where("bound_at >= ? AND bound_at < ?", timeRange.From, timeRange.To).
		Count(&window.RelationAssignedCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("created_at >= ? AND created_at < ?", timeRange.From, timeRange.To).
		Count(&window.AssessmentCreatedCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&evaluationInfra.AssessmentPO{}).
		Where("org_id = ? AND status = ? AND deleted_at IS NULL", orgID, "interpreted").
		Where("interpreted_at >= ? AND interpreted_at < ?", timeRange.From, timeRange.To).
		Count(&window.AssessmentCompletedCount).Error; err != nil {
		return nil, err
	}

	trend := domainStatistics.OrgOverviewTrend{
		Assessments: s.queryDailyCounts(ctx,
			"assessment",
			"created_at",
			"org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ?",
			orgID, timeRange.From, timeRange.To,
		),
		Intakes: s.queryDailyCounts(ctx,
			"clinician_relation",
			"bound_at",
			"org_id = ? AND relation_type = ? AND source_type = ? AND deleted_at IS NULL AND bound_at >= ? AND bound_at < ?",
			orgID, string(actorDomainRelation.RelationTypeCreator), string(actorDomainRelation.SourceTypeAssessmentEntry), timeRange.From, timeRange.To,
		),
		Assignments: s.queryDailyCounts(ctx,
			"clinician_relation",
			"bound_at",
			"org_id = ? AND relation_type IN ? AND deleted_at IS NULL AND bound_at >= ? AND bound_at < ?",
			orgID, accessGrantRelationTypes, timeRange.From, timeRange.To,
		),
	}
	trend.Assessments = fillMissingDailyCounts(timeRange.From, timeRange.To, trend.Assessments)
	trend.Intakes = fillMissingDailyCounts(timeRange.From, timeRange.To, trend.Intakes)
	trend.Assignments = fillMissingDailyCounts(timeRange.From, timeRange.To, trend.Assignments)

	return &domainStatistics.StatisticsOverview{
		OrgID:     orgID,
		TimeRange: timeRange,
		Snapshot:  snapshot,
		Window:    window,
		Trend:     trend,
	}, nil
}

func (s *readService) ListClinicianStatistics(ctx context.Context, orgID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.ClinicianStatisticsList, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page, pageSize)

	var total int64
	if err := s.db.WithContext(ctx).Model(&actorInfra.ClinicianPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&total).Error; err != nil {
		return nil, err
	}

	var clinicians []actorInfra.ClinicianPO
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&clinicians).Error; err != nil {
		return nil, err
	}

	items := make([]*domainStatistics.ClinicianStatistics, 0, len(clinicians))
	for i := range clinicians {
		item, err := s.buildClinicianStatistics(ctx, orgID, clinicians[i], timeRange)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return &domainStatistics.ClinicianStatisticsList{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: calcTotalPages(total, pageSize),
	}, nil
}

func (s *readService) GetClinicianStatistics(ctx context.Context, orgID int64, clinicianID uint64, filter QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	var clinician actorInfra.ClinicianPO
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, clinicianID).
		First(&clinician).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "clinician not found")
		}
		return nil, err
	}

	return s.buildClinicianStatistics(ctx, orgID, clinician, timeRange)
}

func (s *readService) ListAssessmentEntryStatistics(ctx context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}
	page, pageSize = normalizePage(page, pageSize)

	query := s.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).Where("org_id = ? AND deleted_at IS NULL", orgID)
	if clinicianID != nil {
		query = query.Where("clinician_id = ?", *clinicianID)
	}
	if activeOnly != nil {
		query = query.Where("is_active = ?", *activeOnly)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var entries []actorInfra.AssessmentEntryPO
	if err := query.Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&entries).Error; err != nil {
		return nil, err
	}

	items := make([]*domainStatistics.AssessmentEntryStatistics, 0, len(entries))
	for i := range entries {
		item, err := s.buildEntryStatistics(ctx, orgID, entries[i], timeRange)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return &domainStatistics.AssessmentEntryStatisticsList{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: calcTotalPages(total, pageSize),
	}, nil
}

func (s *readService) GetAssessmentEntryStatistics(ctx context.Context, orgID int64, entryID uint64, filter QueryFilter) (*domainStatistics.AssessmentEntryStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}

	var entry actorInfra.AssessmentEntryPO
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, entryID).
		First(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "assessment entry not found")
		}
		return nil, err
	}
	return s.buildEntryStatistics(ctx, orgID, entry, timeRange)
}

func (s *readService) GetCurrentClinicianStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	clinician, err := s.resolveCurrentClinician(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	return s.GetClinicianStatistics(ctx, orgID, clinician.ID.Uint64(), filter)
}

func (s *readService) ListCurrentClinicianEntryStatistics(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	clinician, err := s.resolveCurrentClinician(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}
	clinicianID := clinician.ID.Uint64()
	return s.ListAssessmentEntryStatistics(ctx, orgID, &clinicianID, nil, filter, page, pageSize)
}

func (s *readService) GetCurrentClinicianTesteeSummary(ctx context.Context, orgID int64, operatorUserID int64, filter QueryFilter) (*domainStatistics.ClinicianTesteeSummaryStatistics, error) {
	timeRange, err := normalizeQueryFilter(filter)
	if err != nil {
		return nil, err
	}
	clinician, err := s.resolveCurrentClinician(ctx, orgID, operatorUserID)
	if err != nil {
		return nil, err
	}

	snapshot, err := s.queryClinicianSnapshot(ctx, orgID, clinician.ID.Uint64())
	if err != nil {
		return nil, err
	}

	var keyFocusCount int64
	if err := scanCountQuery(s.db.WithContext(ctx).
		Table("testee t").
		Select("COUNT(DISTINCT t.id) AS count").
		Joins("JOIN clinician_relation r ON r.testee_id = t.id AND r.org_id = t.org_id").
		Where("t.org_id = ? AND t.is_key_focus = ? AND t.deleted_at IS NULL", orgID, true).
		Where("r.clinician_id = ? AND r.is_active = ? AND r.deleted_at IS NULL AND r.relation_type IN ?", clinician.ID, true, accessGrantRelationTypes),
		&keyFocusCount); err != nil {
		return nil, err
	}

	var assessedInWindowCount int64
	if err := scanCountQuery(s.db.WithContext(ctx).
		Table("assessment a").
		Select("COUNT(DISTINCT a.testee_id) AS count").
		Joins("JOIN clinician_relation r ON r.testee_id = a.testee_id AND r.org_id = a.org_id").
		Where("a.org_id = ? AND a.deleted_at IS NULL", orgID).
		Where("r.clinician_id = ? AND r.is_active = ? AND r.deleted_at IS NULL AND r.relation_type IN ?", clinician.ID, true, accessGrantRelationTypes).
		Where("a.created_at >= ? AND a.created_at < ?", timeRange.From, timeRange.To),
		&assessedInWindowCount); err != nil {
		return nil, err
	}

	return &domainStatistics.ClinicianTesteeSummaryStatistics{
		TimeRange:               timeRange,
		TotalAccessibleTestees:  snapshot.TotalAccessibleTestees,
		PrimaryTesteeCount:      snapshot.PrimaryTesteeCount,
		AttendingTesteeCount:    snapshot.AttendingTesteeCount,
		CollaboratorTesteeCount: snapshot.CollaboratorTesteeCount,
		KeyFocusTesteeCount:     keyFocusCount,
		AssessedInWindowCount:   assessedInWindowCount,
	}, nil
}

func (s *readService) GetQuestionnaireBatchStatistics(ctx context.Context, orgID int64, codes []string) (*domainStatistics.QuestionnaireBatchStatisticsResponse, error) {
	cleanCodes := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, codeValue := range codes {
		codeValue = strings.TrimSpace(codeValue)
		if codeValue == "" {
			continue
		}
		if _, exists := seen[codeValue]; exists {
			continue
		}
		seen[codeValue] = struct{}{}
		cleanCodes = append(cleanCodes, codeValue)
	}

	items := make([]*domainStatistics.QuestionnaireBatchStatisticsItem, 0, len(cleanCodes))
	if len(cleanCodes) == 0 {
		return &domainStatistics.QuestionnaireBatchStatisticsResponse{Items: items}, nil
	}

	type row struct {
		Code             string
		TotalSubmissions int64
		TotalCompletions int64
	}

	var rows []row
	if err := s.db.WithContext(ctx).
		Table("assessment").
		Select("COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS code, COUNT(*) AS total_submissions, SUM(CASE WHEN status = 'interpreted' THEN 1 ELSE 0 END) AS total_completions").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) IN ?", cleanCodes).
		Group("COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code)").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	resultByCode := make(map[string]*domainStatistics.QuestionnaireBatchStatisticsItem, len(cleanCodes))
	for _, codeValue := range cleanCodes {
		resultByCode[codeValue] = &domainStatistics.QuestionnaireBatchStatisticsItem{Code: codeValue}
	}
	for _, rowItem := range rows {
		item := resultByCode[rowItem.Code]
		if item == nil {
			item = &domainStatistics.QuestionnaireBatchStatisticsItem{Code: rowItem.Code}
			resultByCode[rowItem.Code] = item
		}
		item.TotalSubmissions = rowItem.TotalSubmissions
		item.TotalCompletions = rowItem.TotalCompletions
		if item.TotalSubmissions > 0 {
			item.CompletionRate = float64(item.TotalCompletions) / float64(item.TotalSubmissions) * 100
		}
	}

	for _, codeValue := range cleanCodes {
		items = append(items, resultByCode[codeValue])
	}

	if s.answerSheetRepo != nil {
		for _, item := range items {
			if item.TotalSubmissions > 0 {
				continue
			}
			count, err := s.answerSheetRepo.CountByQuestionnaire(ctx, item.Code)
			if err != nil {
				return nil, err
			}
			if count <= 0 {
				continue
			}
			item.TotalSubmissions = count
			item.TotalCompletions = count
			item.CompletionRate = 100
		}
	}

	return &domainStatistics.QuestionnaireBatchStatisticsResponse{Items: items}, nil
}

func (s *readService) resolveCurrentClinician(ctx context.Context, orgID int64, operatorUserID int64) (*actorInfra.ClinicianPO, error) {
	var operator actorInfra.OperatorPO
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, operatorUserID, true).
		First(&operator).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "operator is not active in current organization")
		}
		return nil, err
	}

	var clinician actorInfra.ClinicianPO
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND operator_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, operator.ID, true).
		First(&clinician).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrPermissionDenied, "current operator is not bound to an active clinician")
		}
		return nil, err
	}
	return &clinician, nil
}

func (s *readService) buildClinicianStatistics(ctx context.Context, orgID int64, clinician actorInfra.ClinicianPO, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.ClinicianStatistics, error) {
	snapshot, err := s.queryClinicianSnapshot(ctx, orgID, clinician.ID.Uint64())
	if err != nil {
		return nil, err
	}

	window := domainStatistics.ClinicianStatisticsWindow{}
	if err := s.db.WithContext(ctx).Model(&actorInfra.ClinicianRelationPO{}).
		Where("org_id = ? AND clinician_id = ? AND relation_type = ? AND source_type = ? AND deleted_at IS NULL", orgID, clinician.ID, string(actorDomainRelation.RelationTypeCreator), string(actorDomainRelation.SourceTypeAssessmentEntry)).
		Where("bound_at >= ? AND bound_at < ?", timeRange.From, timeRange.To).
		Count(&window.IntakeCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&actorInfra.ClinicianRelationPO{}).
		Where("org_id = ? AND clinician_id = ? AND relation_type IN ? AND deleted_at IS NULL", orgID, clinician.ID, accessGrantRelationTypes).
		Where("bound_at >= ? AND bound_at < ?", timeRange.From, timeRange.To).
		Count(&window.AssignedCount).Error; err != nil {
		return nil, err
	}
	if err := scanCountQuery(s.db.WithContext(ctx).
		Table("assessment a").
		Select("COUNT(DISTINCT a.id) AS count").
		Joins("JOIN clinician_relation r ON r.testee_id = a.testee_id AND r.org_id = a.org_id").
		Where("a.org_id = ? AND a.status = ? AND a.deleted_at IS NULL", orgID, "interpreted").
		Where("r.clinician_id = ? AND r.is_active = ? AND r.deleted_at IS NULL AND r.relation_type IN ?", clinician.ID, true, accessGrantRelationTypes).
		Where("a.interpreted_at >= ? AND a.interpreted_at < ?", timeRange.From, timeRange.To),
		&window.CompletedAssessmentCount); err != nil {
		return nil, err
	}

	funnel := domainStatistics.ClinicianStatisticsFunnel{}
	if err := s.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND clinician_id = ? AND deleted_at IS NULL", orgID, clinician.ID).
		Where("created_at >= ? AND created_at < ?", timeRange.From, timeRange.To).
		Count(&funnel.CreatedCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&statisticsInfra.AssessmentEntryResolveLogPO{}).
		Where("org_id = ? AND clinician_id = ?", orgID, clinician.ID).
		Where("resolved_at >= ? AND resolved_at < ?", timeRange.From, timeRange.To).
		Count(&funnel.ResolvedCount).Error; err != nil {
		return nil, err
	}
	funnel.IntakeCount = window.IntakeCount
	if err := scanCountQuery(s.db.WithContext(ctx).
		Table("clinician_relation").
		Select("COUNT(DISTINCT testee_id) AS count").
		Where("org_id = ? AND clinician_id = ? AND relation_type IN ? AND source_type = ? AND deleted_at IS NULL", orgID, clinician.ID, accessGrantRelationTypes, string(actorDomainRelation.SourceTypeAssessmentEntry)).
		Where("bound_at >= ? AND bound_at < ?", timeRange.From, timeRange.To),
		&funnel.AssignedCount); err != nil {
		return nil, err
	}
	if err := scanCountQuery(s.db.WithContext(ctx).
		Table("clinician_relation r").
		Select("COUNT(DISTINCT r.testee_id) AS count").
		Joins("JOIN assessment a ON a.testee_id = r.testee_id AND a.org_id = r.org_id AND a.deleted_at IS NULL AND a.created_at >= r.bound_at").
		Where("r.org_id = ? AND r.clinician_id = ? AND r.relation_type = ? AND r.source_type = ? AND r.deleted_at IS NULL", orgID, clinician.ID, string(actorDomainRelation.RelationTypeCreator), string(actorDomainRelation.SourceTypeAssessmentEntry)).
		Where("a.created_at >= ? AND a.created_at < ?", timeRange.From, timeRange.To),
		&funnel.AssessmentCount); err != nil {
		return nil, err
	}

	return &domainStatistics.ClinicianStatistics{
		TimeRange: timeRange,
		Clinician: domainStatistics.ClinicianStatisticsSubject{
			ID:            clinician.ID,
			OperatorID:    ptrMetaIDFromUint64(clinician.OperatorID),
			Name:          clinician.Name,
			Department:    clinician.Department,
			Title:         clinician.Title,
			ClinicianType: clinician.ClinicianType,
			IsActive:      clinician.IsActive,
		},
		Snapshot: snapshot,
		Window:   window,
		Funnel:   funnel,
	}, nil
}

func (s *readService) queryClinicianSnapshot(ctx context.Context, orgID int64, clinicianID uint64) (domainStatistics.ClinicianStatisticsSnapshot, error) {
	snapshot := domainStatistics.ClinicianStatisticsSnapshot{}
	countByType := func(relationType string) (int64, error) {
		var count int64
		err := scanCountQuery(s.db.WithContext(ctx).
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
	if err := scanCountQuery(s.db.WithContext(ctx).
		Table("clinician_relation").
		Select("COUNT(DISTINCT testee_id) AS count").
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL AND relation_type IN ?", orgID, clinicianID, true, accessGrantRelationTypes),
		&snapshot.TotalAccessibleTestees); err != nil {
		return snapshot, err
	}
	if err := s.db.WithContext(ctx).Model(&actorInfra.AssessmentEntryPO{}).
		Where("org_id = ? AND clinician_id = ? AND is_active = ? AND deleted_at IS NULL", orgID, clinicianID, true).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Count(&snapshot.ActiveEntryCount).Error; err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (s *readService) buildEntryStatistics(ctx context.Context, orgID int64, entry actorInfra.AssessmentEntryPO, timeRange domainStatistics.StatisticsTimeRange) (*domainStatistics.AssessmentEntryStatistics, error) {
	result := &domainStatistics.AssessmentEntryStatistics{
		TimeRange: timeRange,
		Entry: domainStatistics.AssessmentEntryStatisticsMeta{
			ID:            entry.ID,
			OrgID:         entry.OrgID,
			ClinicianID:   entry.ClinicianID,
			Token:         entry.Token,
			TargetType:    entry.TargetType,
			TargetCode:    entry.TargetCode,
			TargetVersion: derefString(entry.TargetVersion),
			IsActive:      entry.IsActive,
			CreatedAt:     entry.CreatedAt,
			ExpiresAt:     entry.ExpiresAt,
		},
	}

	var clinician actorInfra.ClinicianPO
	if err := s.db.WithContext(ctx).Select("id, name").
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, entry.ClinicianID).
		First(&clinician).Error; err == nil {
		result.Entry.ClinicianName = clinician.Name
	}

	snapshot, err := s.queryEntryCounts(ctx, orgID, entry.ID.Uint64(), nil, nil)
	if err != nil {
		return nil, err
	}
	window, err := s.queryEntryCounts(ctx, orgID, entry.ID.Uint64(), &timeRange.From, &timeRange.To)
	if err != nil {
		return nil, err
	}
	result.Snapshot = snapshot
	result.Window = window

	if err := s.db.WithContext(ctx).
		Model(&statisticsInfra.AssessmentEntryResolveLogPO{}).
		Select("MAX(resolved_at)").
		Where("org_id = ? AND entry_id = ?", orgID, entry.ID.Uint64()).
		Scan(&result.LastResolvedAt).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Model(&actorInfra.ClinicianRelationPO{}).
		Select("MAX(bound_at)").
		Where("org_id = ? AND source_type = ? AND source_id = ? AND relation_type = ? AND deleted_at IS NULL", orgID, string(actorDomainRelation.SourceTypeAssessmentEntry), entry.ID.Uint64(), string(actorDomainRelation.RelationTypeCreator)).
		Scan(&result.LastIntakeAt).Error; err != nil {
		return nil, err
	}

	return result, nil
}

func (s *readService) queryEntryCounts(ctx context.Context, orgID int64, entryID uint64, from, to *time.Time) (domainStatistics.AssessmentEntryStatisticsCounts, error) {
	counts := domainStatistics.AssessmentEntryStatisticsCounts{}

	resolveQuery := s.db.WithContext(ctx).Model(&statisticsInfra.AssessmentEntryResolveLogPO{}).
		Where("org_id = ? AND entry_id = ?", orgID, entryID)
	if from != nil && to != nil {
		resolveQuery = resolveQuery.Where("resolved_at >= ? AND resolved_at < ?", *from, *to)
	}
	if err := resolveQuery.Count(&counts.ResolveCount).Error; err != nil {
		return counts, err
	}

	intakeQuery := s.db.WithContext(ctx).Model(&actorInfra.ClinicianRelationPO{}).
		Where("org_id = ? AND source_type = ? AND source_id = ? AND relation_type = ? AND deleted_at IS NULL", orgID, string(actorDomainRelation.SourceTypeAssessmentEntry), entryID, string(actorDomainRelation.RelationTypeCreator))
	if from != nil && to != nil {
		intakeQuery = intakeQuery.Where("bound_at >= ? AND bound_at < ?", *from, *to)
	}
	if err := intakeQuery.Count(&counts.IntakeCount).Error; err != nil {
		return counts, err
	}

	assignedQuery := s.db.WithContext(ctx).Model(&actorInfra.ClinicianRelationPO{}).
		Where("org_id = ? AND source_type = ? AND source_id = ? AND relation_type IN ? AND deleted_at IS NULL", orgID, string(actorDomainRelation.SourceTypeAssessmentEntry), entryID, accessGrantRelationTypes)
	if from != nil && to != nil {
		assignedQuery = assignedQuery.Where("bound_at >= ? AND bound_at < ?", *from, *to)
	}
	if err := assignedQuery.Count(&counts.AssignedCount).Error; err != nil {
		return counts, err
	}

	assessmentQuery := s.db.WithContext(ctx).
		Table("clinician_relation r").
		Select("COUNT(DISTINCT r.testee_id) AS count").
		Joins("JOIN assessment a ON a.testee_id = r.testee_id AND a.org_id = r.org_id AND a.deleted_at IS NULL AND a.created_at >= r.bound_at").
		Where("r.org_id = ? AND r.source_type = ? AND r.source_id = ? AND r.relation_type = ? AND r.deleted_at IS NULL", orgID, string(actorDomainRelation.SourceTypeAssessmentEntry), entryID, string(actorDomainRelation.RelationTypeCreator))
	if from != nil && to != nil {
		assessmentQuery = assessmentQuery.Where("a.created_at >= ? AND a.created_at < ?", *from, *to)
	}
	if err := scanCountQuery(assessmentQuery, &counts.AssessmentCount); err != nil {
		return counts, err
	}

	return counts, nil
}

func (s *readService) queryDailyCounts(ctx context.Context, tableName, timeColumn, whereClause string, args ...interface{}) []domainStatistics.DailyCount {
	type row struct {
		StatDate time.Time
		Count    int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).
		Table(tableName).
		Select(fmt.Sprintf("DATE(%s) AS stat_date, COUNT(*) AS count", timeColumn)).
		Where(whereClause, args...).
		Group(fmt.Sprintf("DATE(%s)", timeColumn)).
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

func normalizeQueryFilter(filter QueryFilter) (domainStatistics.StatisticsTimeRange, error) {
	now := time.Now()
	preset := strings.TrimSpace(filter.Preset)
	if preset == "" {
		preset = string(domainStatistics.TimeRangePreset30D)
	}

	if strings.TrimSpace(filter.From) != "" || strings.TrimSpace(filter.To) != "" {
		from, err := parseFlexibleTime(filter.From, false)
		if err != nil {
			return domainStatistics.StatisticsTimeRange{}, errors.WithCode(code.ErrInvalidArgument, "invalid from: %v", err)
		}
		to, err := parseFlexibleTime(filter.To, true)
		if err != nil {
			return domainStatistics.StatisticsTimeRange{}, errors.WithCode(code.ErrInvalidArgument, "invalid to: %v", err)
		}
		if !from.Before(to) {
			return domainStatistics.StatisticsTimeRange{}, errors.WithCode(code.ErrInvalidArgument, "from must be before to")
		}
		return domainStatistics.StatisticsTimeRange{
			Preset: domainStatistics.TimeRangePreset(preset),
			From:   from,
			To:     to,
		}, nil
	}

	switch domainStatistics.TimeRangePreset(preset) {
	case domainStatistics.TimeRangePresetToday:
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return domainStatistics.StatisticsTimeRange{
			Preset: domainStatistics.TimeRangePresetToday,
			From:   from,
			To:     now,
		}, nil
	case domainStatistics.TimeRangePreset7D:
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)
		return domainStatistics.StatisticsTimeRange{
			Preset: domainStatistics.TimeRangePreset7D,
			From:   from,
			To:     now,
		}, nil
	case domainStatistics.TimeRangePreset30D:
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -29)
		return domainStatistics.StatisticsTimeRange{
			Preset: domainStatistics.TimeRangePreset30D,
			From:   from,
			To:     now,
		}, nil
	default:
		return domainStatistics.StatisticsTimeRange{}, errors.WithCode(code.ErrInvalidArgument, "unsupported preset: %s", preset)
	}
}

func parseFlexibleTime(raw string, end bool) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		if end {
			return time.Now(), nil
		}
		return time.Time{}, errors.WithCode(code.ErrInvalidArgument, "time is required")
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			if layout == "2006-01-02" && end {
				return t.AddDate(0, 0, 1), nil
			}
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

func fillMissingDailyCounts(from, to time.Time, counts []domainStatistics.DailyCount) []domainStatistics.DailyCount {
	if from.IsZero() || !from.Before(to) {
		return counts
	}

	countMap := make(map[string]int64, len(counts))
	for _, item := range counts {
		countMap[item.Date.Format("2006-01-02")] = item.Count
	}

	filled := make([]domainStatistics.DailyCount, 0)
	cursor := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	endDate := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	if !cursor.Before(endDate) {
		endDate = endDate.AddDate(0, 0, 1)
	}

	for cursor.Before(endDate) {
		key := cursor.Format("2006-01-02")
		filled = append(filled, domainStatistics.DailyCount{
			Date:  cursor,
			Count: countMap[key],
		})
		cursor = cursor.AddDate(0, 0, 1)
	}

	return filled
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func calcTotalPages(total int64, pageSize int) int {
	if total == 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
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

// sortClinicianStatistics 统一排序结果，保留同页内稳定输出。
func sortClinicianStatistics(items []*domainStatistics.ClinicianStatistics) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Clinician.ID > items[j].Clinician.ID
	})
}
