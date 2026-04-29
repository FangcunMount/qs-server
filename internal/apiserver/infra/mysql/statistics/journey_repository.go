package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ domainStatistics.BehaviorFootprintWriter = (*StatisticsRepository)(nil)
var _ domainStatistics.AssessmentEpisodeRepository = (*StatisticsRepository)(nil)
var _ domainStatistics.AnalyticsProjectionRepository = (*StatisticsRepository)(nil)

func (r *StatisticsRepository) AppendBehaviorFootprint(ctx context.Context, footprint *domainStatistics.BehaviorFootprint) error {
	if footprint == nil {
		return nil
	}
	return r.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(behaviorFootprintFromDomain(footprint)).Error
}

func (r *StatisticsRepository) FindLatestFootprintByEvent(
	ctx context.Context,
	orgID int64,
	testeeID uint64,
	eventName domainStatistics.BehaviorEventName,
	occurredAt time.Time,
	window time.Duration,
) (*domainStatistics.BehaviorFootprint, error) {
	var po BehaviorFootprintPO
	query := r.WithContext(ctx).
		Where("org_id = ? AND testee_id = ? AND event_name = ? AND deleted_at IS NULL", orgID, testeeID, string(eventName)).
		Where("occurred_at <= ?", occurredAt)
	if window > 0 {
		query = query.Where("occurred_at >= ?", occurredAt.Add(-window))
	}
	if err := query.Order("occurred_at DESC").First(&po).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return behaviorFootprintToDomain(&po), nil
}

func (r *StatisticsRepository) SaveEpisode(ctx context.Context, episode *domainStatistics.AssessmentEpisode) error {
	if episode == nil {
		return nil
	}
	po := assessmentEpisodeFromDomain(episode)
	if po.EpisodeID == 0 {
		po.EpisodeID = po.AnswerSheetID
	}
	return r.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "episode_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"org_id":                po.OrgID,
			"entry_id":              po.EntryID,
			"clinician_id":          po.ClinicianID,
			"testee_id":             po.TesteeID,
			"answersheet_id":        po.AnswerSheetID,
			"assessment_id":         po.AssessmentID,
			"report_id":             po.ReportID,
			"attributed_intake_at":  po.AttributedIntakeAt,
			"submitted_at":          po.SubmittedAt,
			"assessment_created_at": po.AssessmentCreatedAt,
			"report_generated_at":   po.ReportGeneratedAt,
			"failed_at":             po.FailedAt,
			"status":                po.Status,
			"failure_reason":        po.FailureReason,
		}),
	}).Create(po).Error
}

func (r *StatisticsRepository) FindEpisodeByAnswerSheetID(ctx context.Context, orgID int64, answerSheetID uint64) (*domainStatistics.AssessmentEpisode, error) {
	var po AssessmentEpisodePO
	if err := r.WithContext(ctx).
		Where("org_id = ? AND answersheet_id = ? AND deleted_at IS NULL", orgID, answerSheetID).
		First(&po).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return assessmentEpisodeToDomain(&po), nil
}

func (r *StatisticsRepository) FindEpisodeByAssessmentID(ctx context.Context, orgID int64, assessmentID uint64) (*domainStatistics.AssessmentEpisode, error) {
	var po AssessmentEpisodePO
	if err := r.WithContext(ctx).
		Where("org_id = ? AND assessment_id = ? AND deleted_at IS NULL", orgID, assessmentID).
		First(&po).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return assessmentEpisodeToDomain(&po), nil
}

func (r *StatisticsRepository) ListEpisodesForAttribution(ctx context.Context, orgID int64, testeeID uint64, intakeAt time.Time, window time.Duration) ([]*domainStatistics.AssessmentEpisode, error) {
	var rows []*AssessmentEpisodePO
	query := r.WithContext(ctx).
		Where("org_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, testeeID).
		Where("submitted_at >= ?", intakeAt)
	if window > 0 {
		query = query.Where("submitted_at <= ?", intakeAt.Add(window))
	}
	if err := query.Order("submitted_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]*domainStatistics.AssessmentEpisode, 0, len(rows))
	for _, row := range rows {
		items = append(items, assessmentEpisodeToDomain(row))
	}
	return items, nil
}

func (r *StatisticsRepository) ApplyAnalyticsProjectionMutation(ctx context.Context, mutation domainStatistics.AnalyticsProjectionMutation) error {
	statDate := dateOnly(mutation.StatDate)
	if err := r.upsertAnalyticsOrgProjection(ctx, mutation, statDate); err != nil {
		return err
	}
	if mutation.ClinicianID != 0 {
		if err := r.upsertAnalyticsClinicianProjection(ctx, mutation, statDate); err != nil {
			return err
		}
	}
	if mutation.EntryID != 0 {
		if err := r.upsertAnalyticsEntryProjection(ctx, mutation, statDate); err != nil {
			return err
		}
	}
	return nil
}

func (r *StatisticsRepository) upsertAnalyticsOrgProjection(ctx context.Context, mutation domainStatistics.AnalyticsProjectionMutation, statDate time.Time) error {
	return r.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "org_id"}, {Name: "stat_date"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"entry_opened_count":                  gorm.Expr("entry_opened_count + ?", mutation.EntryOpenedCount),
			"intake_confirmed_count":              gorm.Expr("intake_confirmed_count + ?", mutation.IntakeConfirmedCount),
			"testee_profile_created_count":        gorm.Expr("testee_profile_created_count + ?", mutation.TesteeProfileCreatedCount),
			"care_relationship_established_count": gorm.Expr("care_relationship_established_count + ?", mutation.CareRelationshipEstablishedCount),
			"care_relationship_transferred_count": gorm.Expr("care_relationship_transferred_count + ?", mutation.CareRelationshipTransferredCount),
			"answersheet_submitted_count":         gorm.Expr("answersheet_submitted_count + ?", mutation.AnswerSheetSubmittedCount),
			"assessment_created_count":            gorm.Expr("assessment_created_count + ?", mutation.AssessmentCreatedCount),
			"report_generated_count":              gorm.Expr("report_generated_count + ?", mutation.ReportGeneratedCount),
			"episode_completed_count":             gorm.Expr("episode_completed_count + ?", mutation.EpisodeCompletedCount),
			"episode_failed_count":                gorm.Expr("episode_failed_count + ?", mutation.EpisodeFailedCount),
		}),
	}).Create(&AnalyticsProjectionOrgDailyPO{
		OrgID:                            mutation.OrgID,
		StatDate:                         statDate,
		EntryOpenedCount:                 mutation.EntryOpenedCount,
		IntakeConfirmedCount:             mutation.IntakeConfirmedCount,
		TesteeProfileCreatedCount:        mutation.TesteeProfileCreatedCount,
		CareRelationshipEstablishedCount: mutation.CareRelationshipEstablishedCount,
		CareRelationshipTransferredCount: mutation.CareRelationshipTransferredCount,
		AnswerSheetSubmittedCount:        mutation.AnswerSheetSubmittedCount,
		AssessmentCreatedCount:           mutation.AssessmentCreatedCount,
		ReportGeneratedCount:             mutation.ReportGeneratedCount,
		EpisodeCompletedCount:            mutation.EpisodeCompletedCount,
		EpisodeFailedCount:               mutation.EpisodeFailedCount,
	}).Error
}

func (r *StatisticsRepository) upsertAnalyticsClinicianProjection(ctx context.Context, mutation domainStatistics.AnalyticsProjectionMutation, statDate time.Time) error {
	return r.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "org_id"}, {Name: "clinician_id"}, {Name: "stat_date"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"entry_opened_count":                  gorm.Expr("entry_opened_count + ?", mutation.EntryOpenedCount),
			"intake_confirmed_count":              gorm.Expr("intake_confirmed_count + ?", mutation.IntakeConfirmedCount),
			"testee_profile_created_count":        gorm.Expr("testee_profile_created_count + ?", mutation.TesteeProfileCreatedCount),
			"care_relationship_established_count": gorm.Expr("care_relationship_established_count + ?", mutation.CareRelationshipEstablishedCount),
			"care_relationship_transferred_count": gorm.Expr("care_relationship_transferred_count + ?", mutation.CareRelationshipTransferredCount),
			"answersheet_submitted_count":         gorm.Expr("answersheet_submitted_count + ?", mutation.AnswerSheetSubmittedCount),
			"assessment_created_count":            gorm.Expr("assessment_created_count + ?", mutation.AssessmentCreatedCount),
			"report_generated_count":              gorm.Expr("report_generated_count + ?", mutation.ReportGeneratedCount),
			"episode_completed_count":             gorm.Expr("episode_completed_count + ?", mutation.EpisodeCompletedCount),
			"episode_failed_count":                gorm.Expr("episode_failed_count + ?", mutation.EpisodeFailedCount),
		}),
	}).Create(&AnalyticsProjectionClinicianDailyPO{
		OrgID:                            mutation.OrgID,
		ClinicianID:                      mutation.ClinicianID,
		StatDate:                         statDate,
		EntryOpenedCount:                 mutation.EntryOpenedCount,
		IntakeConfirmedCount:             mutation.IntakeConfirmedCount,
		TesteeProfileCreatedCount:        mutation.TesteeProfileCreatedCount,
		CareRelationshipEstablishedCount: mutation.CareRelationshipEstablishedCount,
		CareRelationshipTransferredCount: mutation.CareRelationshipTransferredCount,
		AnswerSheetSubmittedCount:        mutation.AnswerSheetSubmittedCount,
		AssessmentCreatedCount:           mutation.AssessmentCreatedCount,
		ReportGeneratedCount:             mutation.ReportGeneratedCount,
		EpisodeCompletedCount:            mutation.EpisodeCompletedCount,
		EpisodeFailedCount:               mutation.EpisodeFailedCount,
	}).Error
}

func (r *StatisticsRepository) ApplyAnalyticsClinicianProjectionMutation(ctx context.Context, mutation domainStatistics.AnalyticsProjectionMutation) error {
	if mutation.ClinicianID == 0 {
		return nil
	}
	return r.upsertAnalyticsClinicianProjection(ctx, mutation, dateOnly(mutation.StatDate))
}

func (r *StatisticsRepository) upsertAnalyticsEntryProjection(ctx context.Context, mutation domainStatistics.AnalyticsProjectionMutation, statDate time.Time) error {
	return r.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "org_id"}, {Name: "entry_id"}, {Name: "stat_date"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"clinician_id":                        mutation.ClinicianID,
			"entry_opened_count":                  gorm.Expr("entry_opened_count + ?", mutation.EntryOpenedCount),
			"intake_confirmed_count":              gorm.Expr("intake_confirmed_count + ?", mutation.IntakeConfirmedCount),
			"testee_profile_created_count":        gorm.Expr("testee_profile_created_count + ?", mutation.TesteeProfileCreatedCount),
			"care_relationship_established_count": gorm.Expr("care_relationship_established_count + ?", mutation.CareRelationshipEstablishedCount),
			"care_relationship_transferred_count": gorm.Expr("care_relationship_transferred_count + ?", mutation.CareRelationshipTransferredCount),
			"answersheet_submitted_count":         gorm.Expr("answersheet_submitted_count + ?", mutation.AnswerSheetSubmittedCount),
			"assessment_created_count":            gorm.Expr("assessment_created_count + ?", mutation.AssessmentCreatedCount),
			"report_generated_count":              gorm.Expr("report_generated_count + ?", mutation.ReportGeneratedCount),
			"episode_completed_count":             gorm.Expr("episode_completed_count + ?", mutation.EpisodeCompletedCount),
			"episode_failed_count":                gorm.Expr("episode_failed_count + ?", mutation.EpisodeFailedCount),
		}),
	}).Create(&AnalyticsProjectionEntryDailyPO{
		OrgID:                            mutation.OrgID,
		EntryID:                          mutation.EntryID,
		ClinicianID:                      mutation.ClinicianID,
		StatDate:                         statDate,
		EntryOpenedCount:                 mutation.EntryOpenedCount,
		IntakeConfirmedCount:             mutation.IntakeConfirmedCount,
		TesteeProfileCreatedCount:        mutation.TesteeProfileCreatedCount,
		CareRelationshipEstablishedCount: mutation.CareRelationshipEstablishedCount,
		CareRelationshipTransferredCount: mutation.CareRelationshipTransferredCount,
		AnswerSheetSubmittedCount:        mutation.AnswerSheetSubmittedCount,
		AssessmentCreatedCount:           mutation.AssessmentCreatedCount,
		ReportGeneratedCount:             mutation.ReportGeneratedCount,
		EpisodeCompletedCount:            mutation.EpisodeCompletedCount,
		EpisodeFailedCount:               mutation.EpisodeFailedCount,
	}).Error
}

func (r *StatisticsRepository) ApplyAnalyticsEntryProjectionMutation(ctx context.Context, mutation domainStatistics.AnalyticsProjectionMutation) error {
	if mutation.EntryID == 0 {
		return nil
	}
	return r.upsertAnalyticsEntryProjection(ctx, mutation, dateOnly(mutation.StatDate))
}

func dateOnly(v time.Time) time.Time {
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, v.Location())
}

func (r *StatisticsRepository) TryBeginAnalyticsProjectorCheckpoint(ctx context.Context, eventID, eventType string) (string, error) {
	if eventID == "" {
		return "", nil
	}
	po := &AnalyticsProjectorCheckpointPO{
		EventID:   eventID,
		EventType: eventType,
		Status:    AnalyticsProjectorCheckpointStatusProcessing,
	}
	if err := r.WithContext(ctx).Create(po).Error; err == nil {
		return "", nil
	}
	var existing AnalyticsProjectorCheckpointPO
	if err := r.WithContext(ctx).
		Where("event_id = ? AND deleted_at IS NULL", eventID).
		First(&existing).Error; err != nil {
		return "", err
	}
	return existing.Status, nil
}

func (r *StatisticsRepository) MarkAnalyticsProjectorCheckpointStatus(ctx context.Context, eventID, status string) error {
	if eventID == "" {
		return nil
	}
	return r.WithContext(ctx).
		Model(&AnalyticsProjectorCheckpointPO{}).
		Where("event_id = ?", eventID).
		Update("status", status).Error
}

func (r *StatisticsRepository) UpsertAnalyticsPendingEvent(ctx context.Context, eventID, eventType, payload string, nextAttemptAt time.Time, lastError string) error {
	if eventID == "" {
		return nil
	}
	return r.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"event_type":      eventType,
			"payload_json":    payload,
			"next_attempt_at": nextAttemptAt,
			"last_error":      lastError,
			"attempt_count":   gorm.Expr("attempt_count + 1"),
		}),
	}).Create(&AnalyticsPendingEventPO{
		EventID:       eventID,
		EventType:     eventType,
		PayloadJSON:   payload,
		NextAttemptAt: nextAttemptAt,
		LastError:     lastError,
		AttemptCount:  1,
	}).Error
}

func (r *StatisticsRepository) ListDueAnalyticsPendingEvents(ctx context.Context, limit int, now time.Time) ([]*domainStatistics.AnalyticsPendingEvent, error) {
	var rows []*AnalyticsPendingEventPO
	query := r.WithContext(ctx).
		Where("next_attempt_at <= ? AND deleted_at IS NULL", now).
		Order("next_attempt_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]*domainStatistics.AnalyticsPendingEvent, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		items = append(items, &domainStatistics.AnalyticsPendingEvent{
			EventID:      row.EventID,
			EventType:    row.EventType,
			PayloadJSON:  row.PayloadJSON,
			AttemptCount: row.AttemptCount,
		})
	}
	return items, nil
}

func (r *StatisticsRepository) RescheduleAnalyticsPendingEvent(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error {
	return r.WithContext(ctx).
		Model(&AnalyticsPendingEventPO{}).
		Where("event_id = ?", eventID).
		Updates(map[string]interface{}{
			"last_error":      lastError,
			"next_attempt_at": nextAttemptAt,
			"attempt_count":   gorm.Expr("attempt_count + 1"),
		}).Error
}

func (r *StatisticsRepository) DeleteAnalyticsPendingEvent(ctx context.Context, eventID string) error {
	if eventID == "" {
		return nil
	}
	return r.WithContext(ctx).Where("event_id = ?", eventID).Delete(&AnalyticsPendingEventPO{}).Error
}
