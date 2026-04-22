package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/pkg/event"
	"gorm.io/gorm"
)

const (
	behaviorAttributionWindow     = 30 * 24 * time.Hour
	defaultBehaviorPendingBackoff = 10 * time.Second
	maxBehaviorPendingBackoff     = 5 * time.Minute
)

type BehaviorProjectEventStatus string

const (
	BehaviorProjectEventStatusCompleted BehaviorProjectEventStatus = "completed"
	BehaviorProjectEventStatusPending   BehaviorProjectEventStatus = "pending"
)

type BehaviorProjectEventInput struct {
	EventID           string    `json:"event_id"`
	EventType         string    `json:"event_type"`
	OrgID             int64     `json:"org_id"`
	ClinicianID       uint64    `json:"clinician_id,omitempty"`
	SourceClinicianID uint64    `json:"source_clinician_id,omitempty"`
	EntryID           uint64    `json:"entry_id,omitempty"`
	TesteeID          uint64    `json:"testee_id,omitempty"`
	AnswerSheetID     uint64    `json:"answersheet_id,omitempty"`
	AssessmentID      uint64    `json:"assessment_id,omitempty"`
	ReportID          uint64    `json:"report_id,omitempty"`
	FailureReason     string    `json:"failure_reason,omitempty"`
	OccurredAt        time.Time `json:"occurred_at"`
}

type BehaviorProjectEventResult struct {
	Status BehaviorProjectEventStatus
}

type BehaviorProjectorService interface {
	ProjectBehaviorEvent(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventResult, error)
	ReconcilePendingBehaviorEvents(ctx context.Context, limit int) (int, error)
}

type behaviorEventStager struct {
	outboxStore behaviorEventOutboxStore
}

type behaviorEventOutboxStore interface {
	StageEventsTx(tx *gorm.DB, events []event.DomainEvent) error
}

func NewBehaviorEventStager(outboxStore behaviorEventOutboxStore) interface {
	assessmentEntryApp.BehaviorEventStager
	clinicianApp.BehaviorEventStager
} {
	if outboxStore == nil {
		return nil
	}
	return &behaviorEventStager{outboxStore: outboxStore}
}

func (s *behaviorEventStager) stage(ctx context.Context, evt event.DomainEvent) error {
	if s == nil || s.outboxStore == nil || evt == nil {
		return nil
	}
	tx, ok := mysql.TxFromContext(ctx)
	if !ok {
		return fmt.Errorf("behavior event staging requires mysql transaction")
	}
	return s.outboxStore.StageEventsTx(tx, []event.DomainEvent{evt})
}

func (s *behaviorEventStager) StageEntryOpened(ctx context.Context, orgID int64, clinicianID, entryID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintEntryOpenedEvent(orgID, clinicianID, entryID, occurredAt))
}

func (s *behaviorEventStager) StageIntakeConfirmed(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintIntakeConfirmedEvent(orgID, clinicianID, entryID, testeeID, occurredAt))
}

func (s *behaviorEventStager) StageTesteeProfileCreated(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintTesteeProfileCreatedEvent(orgID, clinicianID, entryID, testeeID, occurredAt))
}

func (s *behaviorEventStager) StageCareRelationshipEstablished(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintCareRelationshipEstablishedEvent(orgID, clinicianID, entryID, testeeID, occurredAt))
}

func (s *behaviorEventStager) StageCareRelationshipTransferred(ctx context.Context, orgID int64, fromClinicianID, toClinicianID, testeeID uint64, occurredAt time.Time) error {
	return s.stage(ctx, domainStatistics.NewFootprintCareRelationshipTransferredEvent(orgID, fromClinicianID, toClinicianID, testeeID, occurredAt))
}

type assessmentEpisodeProjector struct {
	db   *gorm.DB
	repo *statisticsInfra.StatisticsRepository
}

func NewAssessmentEpisodeProjector(db *gorm.DB, repo *statisticsInfra.StatisticsRepository) BehaviorProjectorService {
	if db == nil || repo == nil {
		return nil
	}
	return &assessmentEpisodeProjector{db: db, repo: repo}
}

func (p *assessmentEpisodeProjector) ProjectBehaviorEvent(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventResult, error) {
	result := BehaviorProjectEventResult{Status: BehaviorProjectEventStatusCompleted}
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := mysql.WithTx(ctx, tx)
		existing, err := p.repo.TryBeginAnalyticsProjectorCheckpoint(txCtx, input.EventID, input.EventType)
		if err != nil {
			return err
		}
		if existing != "" {
			if existing == statisticsInfra.AnalyticsProjectorCheckpointStatusPending {
				result.Status = BehaviorProjectEventStatusPending
			}
			return nil
		}

		status, err := p.projectEvent(txCtx, input)
		if err != nil {
			return err
		}
		if status == BehaviorProjectEventStatusPending {
			result.Status = status
			payload, err := marshalBehaviorProjectEventInput(input)
			if err != nil {
				return err
			}
			if err := p.repo.UpsertAnalyticsPendingEvent(txCtx, input.EventID, input.EventType, payload, time.Now().Add(nextBehaviorPendingBackoff(1)), "pending_attribution"); err != nil {
				return err
			}
			return p.repo.MarkAnalyticsProjectorCheckpointStatus(txCtx, input.EventID, statisticsInfra.AnalyticsProjectorCheckpointStatusPending)
		}
		return p.repo.MarkAnalyticsProjectorCheckpointStatus(txCtx, input.EventID, statisticsInfra.AnalyticsProjectorCheckpointStatusCompleted)
	})
	return result, err
}

func (p *assessmentEpisodeProjector) ReconcilePendingBehaviorEvents(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.repo.ListDueAnalyticsPendingEvents(ctx, limit, time.Now())
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range rows {
		if item == nil {
			continue
		}
		var input BehaviorProjectEventInput
		if err := json.Unmarshal([]byte(item.PayloadJSON), &input); err != nil {
			if txErr := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				txCtx := mysql.WithTx(ctx, tx)
				return p.repo.RescheduleAnalyticsPendingEvent(txCtx, item.EventID, err.Error(), time.Now().Add(nextBehaviorPendingBackoff(item.AttemptCount+1)))
			}); txErr != nil {
				return processed, txErr
			}
			continue
		}

		err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txCtx := mysql.WithTx(ctx, tx)
			status, err := p.projectEvent(txCtx, input)
			if err != nil {
				return p.repo.RescheduleAnalyticsPendingEvent(txCtx, input.EventID, err.Error(), time.Now().Add(nextBehaviorPendingBackoff(item.AttemptCount+1)))
			}
			if status == BehaviorProjectEventStatusPending {
				return p.repo.RescheduleAnalyticsPendingEvent(txCtx, input.EventID, "pending_attribution", time.Now().Add(nextBehaviorPendingBackoff(item.AttemptCount+1)))
			}
			if err := p.repo.DeleteAnalyticsPendingEvent(txCtx, input.EventID); err != nil {
				return err
			}
			return p.repo.MarkAnalyticsProjectorCheckpointStatus(txCtx, input.EventID, statisticsInfra.AnalyticsProjectorCheckpointStatusCompleted)
		})
		if err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (p *assessmentEpisodeProjector) projectEvent(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventStatus, error) {
	switch input.EventType {
	case domainStatistics.EventTypeFootprintEntryOpened:
		return BehaviorProjectEventStatusCompleted, p.applyEntryOpened(ctx, input)
	case domainStatistics.EventTypeFootprintIntakeConfirmed:
		return BehaviorProjectEventStatusCompleted, p.applyIntakeConfirmed(ctx, input)
	case domainStatistics.EventTypeFootprintTesteeProfileCreated:
		return BehaviorProjectEventStatusCompleted, p.applyTesteeProfileCreated(ctx, input)
	case domainStatistics.EventTypeFootprintCareRelationshipEstablished:
		return BehaviorProjectEventStatusCompleted, p.applyCareRelationshipEstablished(ctx, input)
	case domainStatistics.EventTypeFootprintCareRelationshipTransferred:
		return BehaviorProjectEventStatusCompleted, p.applyCareRelationshipTransferred(ctx, input)
	case domainStatistics.EventTypeFootprintAnswerSheetSubmitted:
		return BehaviorProjectEventStatusCompleted, p.applyAnswerSheetSubmitted(ctx, input)
	case domainStatistics.EventTypeFootprintAssessmentCreated:
		return p.applyAssessmentCreated(ctx, input)
	case domainStatistics.EventTypeFootprintReportGenerated:
		return p.applyReportGenerated(ctx, input)
	case domainAssessment.EventTypeFailed:
		return p.applyAssessmentFailed(ctx, input)
	default:
		return BehaviorProjectEventStatusCompleted, fmt.Errorf("unsupported behavior event type %q", input.EventType)
	}
}

func (p *assessmentEpisodeProjector) appendBehaviorFootprint(ctx context.Context, input BehaviorProjectEventInput, eventName domainStatistics.BehaviorEventName, subjectType string, subjectID uint64, actorType string, actorID uint64) error {
	return p.repo.AppendBehaviorFootprint(ctx, &domainStatistics.BehaviorFootprint{
		ID:                input.EventID,
		OrgID:             input.OrgID,
		SubjectType:       subjectType,
		SubjectID:         subjectID,
		ActorType:         actorType,
		ActorID:           actorID,
		EntryID:           input.EntryID,
		ClinicianID:       input.ClinicianID,
		SourceClinicianID: input.SourceClinicianID,
		TesteeID:          input.TesteeID,
		AnswerSheetID:     input.AnswerSheetID,
		AssessmentID:      input.AssessmentID,
		ReportID:          input.ReportID,
		EventName:         eventName,
		OccurredAt:        input.OccurredAt,
		Properties: map[string]interface{}{
			"source_event_id": input.EventID,
			"source_event":    input.EventType,
		},
	})
}

func (p *assessmentEpisodeProjector) applyEntryOpened(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := p.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventEntryOpened, "assessment_entry", input.EntryID, "assessment_entry", input.EntryID); err != nil {
		return err
	}
	return p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:            input.OrgID,
		ClinicianID:      input.ClinicianID,
		EntryID:          input.EntryID,
		StatDate:         input.OccurredAt,
		EntryOpenedCount: 1,
	})
}

func (p *assessmentEpisodeProjector) applyIntakeConfirmed(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := p.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventIntakeConfirmed, "testee", input.TesteeID, "clinician", input.ClinicianID); err != nil {
		return err
	}
	if err := p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                input.OrgID,
		ClinicianID:          input.ClinicianID,
		EntryID:              input.EntryID,
		StatDate:             input.OccurredAt,
		IntakeConfirmedCount: 1,
	}); err != nil {
		return err
	}
	return p.rebindEpisodesToIntake(ctx, input)
}

func (p *assessmentEpisodeProjector) applyTesteeProfileCreated(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := p.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventTesteeProfileCreated, "testee", input.TesteeID, "clinician", input.ClinicianID); err != nil {
		return err
	}
	return p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                     input.OrgID,
		ClinicianID:               input.ClinicianID,
		EntryID:                   input.EntryID,
		StatDate:                  input.OccurredAt,
		TesteeProfileCreatedCount: 1,
	})
}

func (p *assessmentEpisodeProjector) applyCareRelationshipEstablished(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := p.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventCareRelationshipEstablished, "testee", input.TesteeID, "clinician", input.ClinicianID); err != nil {
		return err
	}
	return p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                            input.OrgID,
		ClinicianID:                      input.ClinicianID,
		EntryID:                          input.EntryID,
		StatDate:                         input.OccurredAt,
		CareRelationshipEstablishedCount: 1,
	})
}

func (p *assessmentEpisodeProjector) applyCareRelationshipTransferred(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := p.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventCareRelationshipTransferred, "testee", input.TesteeID, "clinician", input.ClinicianID); err != nil {
		return err
	}
	return p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                            input.OrgID,
		ClinicianID:                      input.ClinicianID,
		StatDate:                         input.OccurredAt,
		CareRelationshipTransferredCount: 1,
	})
}

func (p *assessmentEpisodeProjector) applyAnswerSheetSubmitted(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := p.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventAnswerSheetSubmitted, "answersheet", input.AnswerSheetID, "testee", input.TesteeID); err != nil {
		return err
	}
	episode, err := p.repo.FindEpisodeByAnswerSheetID(ctx, input.OrgID, input.AnswerSheetID)
	if err != nil {
		return err
	}
	if episode == nil {
		episode = &domainStatistics.AssessmentEpisode{
			EpisodeID:           input.AnswerSheetID,
			OrgID:               input.OrgID,
			TesteeID:            input.TesteeID,
			AnswerSheetID:       input.AnswerSheetID,
			SubmittedAt:         input.OccurredAt,
			Status:              domainStatistics.EpisodeStatusActive,
			AttributedIntakeAt:  nil,
			AssessmentCreatedAt: nil,
			ReportGeneratedAt:   nil,
			FailedAt:            nil,
		}
		if latestIntake, err := p.repo.FindLatestFootprintByEvent(ctx, input.OrgID, input.TesteeID, domainStatistics.BehaviorEventIntakeConfirmed, input.OccurredAt, behaviorAttributionWindow); err != nil {
			return err
		} else if latestIntake != nil {
			if latestIntake.EntryID != 0 {
				episode.EntryID = uint64Ptr(latestIntake.EntryID)
			}
			if latestIntake.ClinicianID != 0 {
				episode.ClinicianID = uint64Ptr(latestIntake.ClinicianID)
			}
			episode.AttributedIntakeAt = timePtr(latestIntake.OccurredAt)
		}
		if err := p.repo.SaveEpisode(ctx, episode); err != nil {
			return err
		}
	}
	return p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                     input.OrgID,
		ClinicianID:               valueOrZero(episode.ClinicianID),
		EntryID:                   valueOrZero(episode.EntryID),
		StatDate:                  input.OccurredAt,
		AnswerSheetSubmittedCount: 1,
	})
}

func (p *assessmentEpisodeProjector) applyAssessmentCreated(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventStatus, error) {
	if err := p.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventAssessmentCreated, "assessment", input.AssessmentID, "testee", input.TesteeID); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	episode, err := p.repo.FindEpisodeByAnswerSheetID(ctx, input.OrgID, input.AnswerSheetID)
	if err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	if episode == nil {
		return BehaviorProjectEventStatusPending, nil
	}
	if episode.AssessmentID != nil && *episode.AssessmentID == input.AssessmentID && episode.AssessmentCreatedAt != nil {
		return BehaviorProjectEventStatusCompleted, nil
	}
	episode.AssessmentID = uint64Ptr(input.AssessmentID)
	episode.AssessmentCreatedAt = timePtr(input.OccurredAt)
	if err := p.repo.SaveEpisode(ctx, episode); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	if err := p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                  input.OrgID,
		ClinicianID:            valueOrZero(episode.ClinicianID),
		EntryID:                valueOrZero(episode.EntryID),
		StatDate:               input.OccurredAt,
		AssessmentCreatedCount: 1,
	}); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	return BehaviorProjectEventStatusCompleted, nil
}

func (p *assessmentEpisodeProjector) applyReportGenerated(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventStatus, error) {
	if err := p.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventReportGenerated, "report", input.ReportID, "assessment", input.AssessmentID); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	episode, err := p.repo.FindEpisodeByAssessmentID(ctx, input.OrgID, input.AssessmentID)
	if err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	if episode == nil {
		return BehaviorProjectEventStatusPending, nil
	}
	if episode.ReportID != nil && *episode.ReportID == input.ReportID && episode.ReportGeneratedAt != nil {
		return BehaviorProjectEventStatusCompleted, nil
	}
	episode.ReportID = uint64Ptr(input.ReportID)
	episode.ReportGeneratedAt = timePtr(input.OccurredAt)
	episode.Status = domainStatistics.EpisodeStatusCompleted
	if err := p.repo.SaveEpisode(ctx, episode); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	if err := p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                 input.OrgID,
		ClinicianID:           valueOrZero(episode.ClinicianID),
		EntryID:               valueOrZero(episode.EntryID),
		StatDate:              input.OccurredAt,
		ReportGeneratedCount:  1,
		EpisodeCompletedCount: 1,
	}); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	return BehaviorProjectEventStatusCompleted, nil
}

func (p *assessmentEpisodeProjector) applyAssessmentFailed(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventStatus, error) {
	episode, err := p.repo.FindEpisodeByAssessmentID(ctx, input.OrgID, input.AssessmentID)
	if err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	if episode == nil {
		return BehaviorProjectEventStatusPending, nil
	}
	if episode.Status == domainStatistics.EpisodeStatusFailed {
		return BehaviorProjectEventStatusCompleted, nil
	}
	episode.Status = domainStatistics.EpisodeStatusFailed
	episode.FailureReason = input.FailureReason
	episode.FailedAt = timePtr(input.OccurredAt)
	if err := p.repo.SaveEpisode(ctx, episode); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	if err := p.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:              input.OrgID,
		ClinicianID:        valueOrZero(episode.ClinicianID),
		EntryID:            valueOrZero(episode.EntryID),
		StatDate:           input.OccurredAt,
		EpisodeFailedCount: 1,
	}); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	return BehaviorProjectEventStatusCompleted, nil
}

func marshalBehaviorProjectEventInput(input BehaviorProjectEventInput) (string, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func timePtr(v time.Time) *time.Time {
	return &v
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func valueOrZero(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}

func nextBehaviorPendingBackoff(attemptCount int64) time.Duration {
	if attemptCount <= 1 {
		return defaultBehaviorPendingBackoff
	}
	backoff := defaultBehaviorPendingBackoff
	for i := int64(1); i < attemptCount && backoff < maxBehaviorPendingBackoff; i++ {
		backoff *= 2
		if backoff >= maxBehaviorPendingBackoff {
			return maxBehaviorPendingBackoff
		}
	}
	return backoff
}

func (p *assessmentEpisodeProjector) rebindEpisodesToIntake(ctx context.Context, input BehaviorProjectEventInput) error {
	episodes, err := p.repo.ListEpisodesForAttribution(ctx, input.OrgID, input.TesteeID, input.OccurredAt, behaviorAttributionWindow)
	if err != nil {
		return err
	}
	for _, episode := range episodes {
		if episode == nil || input.OccurredAt.After(episode.SubmittedAt) {
			continue
		}
		oldEntryID := valueOrZero(episode.EntryID)
		oldClinicianID := valueOrZero(episode.ClinicianID)
		if episode.AttributedIntakeAt != nil && !input.OccurredAt.After(*episode.AttributedIntakeAt) {
			continue
		}
		episode.EntryID = uint64Ptr(input.EntryID)
		episode.ClinicianID = uint64Ptr(input.ClinicianID)
		episode.AttributedIntakeAt = timePtr(input.OccurredAt)
		if err := p.repo.SaveEpisode(ctx, episode); err != nil {
			return err
		}
		if err := p.reallocateEpisodeProjection(ctx, episode, oldClinicianID, oldEntryID, input.ClinicianID, input.EntryID); err != nil {
			return err
		}
	}
	return nil
}

func (p *assessmentEpisodeProjector) reallocateEpisodeProjection(ctx context.Context, episode *domainStatistics.AssessmentEpisode, oldClinicianID, oldEntryID, newClinicianID, newEntryID uint64) error {
	mutations := episodeProjectionMutations(episode)
	if oldClinicianID == newClinicianID && oldEntryID == newEntryID {
		return nil
	}
	for _, mutation := range mutations {
		if oldClinicianID != 0 && oldClinicianID != newClinicianID {
			negative := mutation
			negative.ClinicianID = oldClinicianID
			negative.EntryID = 0
			invertAnalyticsProjectionMutation(&negative)
			if err := p.repo.ApplyAnalyticsClinicianProjectionMutation(ctx, negative); err != nil {
				return err
			}
		}
		if oldEntryID != 0 && oldEntryID != newEntryID {
			negative := mutation
			negative.ClinicianID = oldClinicianID
			negative.EntryID = oldEntryID
			invertAnalyticsProjectionMutation(&negative)
			if err := p.repo.ApplyAnalyticsEntryProjectionMutation(ctx, negative); err != nil {
				return err
			}
		}
		if newClinicianID != 0 && oldClinicianID != newClinicianID {
			positive := mutation
			positive.ClinicianID = newClinicianID
			positive.EntryID = 0
			if err := p.repo.ApplyAnalyticsClinicianProjectionMutation(ctx, positive); err != nil {
				return err
			}
		}
		if newEntryID != 0 && oldEntryID != newEntryID {
			positive := mutation
			positive.ClinicianID = newClinicianID
			positive.EntryID = newEntryID
			if err := p.repo.ApplyAnalyticsEntryProjectionMutation(ctx, positive); err != nil {
				return err
			}
		}
	}
	return nil
}

func episodeProjectionMutations(episode *domainStatistics.AssessmentEpisode) []domainStatistics.AnalyticsProjectionMutation {
	if episode == nil {
		return nil
	}
	mutations := []domainStatistics.AnalyticsProjectionMutation{{
		OrgID:                     episode.OrgID,
		StatDate:                  episode.SubmittedAt,
		AnswerSheetSubmittedCount: 1,
	}}
	if episode.AssessmentCreatedAt != nil {
		mutations = append(mutations, domainStatistics.AnalyticsProjectionMutation{
			OrgID:                  episode.OrgID,
			StatDate:               *episode.AssessmentCreatedAt,
			AssessmentCreatedCount: 1,
		})
	}
	if episode.ReportGeneratedAt != nil {
		mutations = append(mutations, domainStatistics.AnalyticsProjectionMutation{
			OrgID:                 episode.OrgID,
			StatDate:              *episode.ReportGeneratedAt,
			ReportGeneratedCount:  1,
			EpisodeCompletedCount: 1,
		})
	}
	if episode.Status == domainStatistics.EpisodeStatusFailed && episode.FailedAt != nil {
		mutations = append(mutations, domainStatistics.AnalyticsProjectionMutation{
			OrgID:              episode.OrgID,
			StatDate:           *episode.FailedAt,
			EpisodeFailedCount: 1,
		})
	}
	return mutations
}

func invertAnalyticsProjectionMutation(mutation *domainStatistics.AnalyticsProjectionMutation) {
	if mutation == nil {
		return
	}
	mutation.EntryOpenedCount *= -1
	mutation.IntakeConfirmedCount *= -1
	mutation.TesteeProfileCreatedCount *= -1
	mutation.CareRelationshipEstablishedCount *= -1
	mutation.CareRelationshipTransferredCount *= -1
	mutation.AnswerSheetSubmittedCount *= -1
	mutation.AssessmentCreatedCount *= -1
	mutation.ReportGeneratedCount *= -1
	mutation.EpisodeCompletedCount *= -1
	mutation.EpisodeFailedCount *= -1
}
