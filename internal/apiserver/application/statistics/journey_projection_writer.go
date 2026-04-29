package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type projectionWriter struct {
	repo BehaviorProjectionRepository
}

func (w projectionWriter) appendBehaviorFootprint(ctx context.Context, input BehaviorProjectEventInput, eventName domainStatistics.BehaviorEventName, subjectType string, subjectID uint64, actorType string, actorID uint64) error {
	return w.repo.AppendBehaviorFootprint(ctx, &domainStatistics.BehaviorFootprint{
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

func (w projectionWriter) reallocateEpisodeProjection(ctx context.Context, episode *domainStatistics.AssessmentEpisode, oldClinicianID, oldEntryID, newClinicianID, newEntryID uint64) error {
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
			if err := w.repo.ApplyAnalyticsClinicianProjectionMutation(ctx, negative); err != nil {
				return err
			}
		}
		if oldEntryID != 0 && oldEntryID != newEntryID {
			negative := mutation
			negative.ClinicianID = oldClinicianID
			negative.EntryID = oldEntryID
			invertAnalyticsProjectionMutation(&negative)
			if err := w.repo.ApplyAnalyticsEntryProjectionMutation(ctx, negative); err != nil {
				return err
			}
		}
		if newClinicianID != 0 && oldClinicianID != newClinicianID {
			positive := mutation
			positive.ClinicianID = newClinicianID
			positive.EntryID = 0
			if err := w.repo.ApplyAnalyticsClinicianProjectionMutation(ctx, positive); err != nil {
				return err
			}
		}
		if newEntryID != 0 && oldEntryID != newEntryID {
			positive := mutation
			positive.ClinicianID = newClinicianID
			positive.EntryID = newEntryID
			if err := w.repo.ApplyAnalyticsEntryProjectionMutation(ctx, positive); err != nil {
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
	if episode.ReportGeneratedAt != nil {
		mutations = append(mutations, domainStatistics.AnalyticsProjectionMutation{
			OrgID:                  episode.OrgID,
			StatDate:               *episode.ReportGeneratedAt,
			AssessmentCreatedCount: 1,
			ReportGeneratedCount:   1,
			EpisodeCompletedCount:  1,
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
