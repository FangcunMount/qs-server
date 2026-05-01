package statistics

import (
	"context"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type episodeLifecycler struct {
	repo       BehaviorProjectionRepository
	projection projectionWriter
}

func (l episodeLifecycler) applyEntryOpened(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := l.projection.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventEntryOpened, "assessment_entry", input.EntryID, "assessment_entry", input.EntryID); err != nil {
		return err
	}
	return l.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:            input.OrgID,
		ClinicianID:      input.ClinicianID,
		EntryID:          input.EntryID,
		StatDate:         input.OccurredAt,
		EntryOpenedCount: 1,
	})
}

func (l episodeLifecycler) applyIntakeConfirmed(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := l.projection.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventIntakeConfirmed, "testee", input.TesteeID, "clinician", input.ClinicianID); err != nil {
		return err
	}
	if err := l.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                input.OrgID,
		ClinicianID:          input.ClinicianID,
		EntryID:              input.EntryID,
		StatDate:             input.OccurredAt,
		IntakeConfirmedCount: 1,
	}); err != nil {
		return err
	}
	return l.rebindEpisodesToIntake(ctx, input)
}

func (l episodeLifecycler) applyTesteeProfileCreated(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := l.projection.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventTesteeProfileCreated, "testee", input.TesteeID, "clinician", input.ClinicianID); err != nil {
		return err
	}
	return l.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                     input.OrgID,
		ClinicianID:               input.ClinicianID,
		EntryID:                   input.EntryID,
		StatDate:                  input.OccurredAt,
		TesteeProfileCreatedCount: 1,
	})
}

func (l episodeLifecycler) applyCareRelationshipEstablished(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := l.projection.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventCareRelationshipEstablished, "testee", input.TesteeID, "clinician", input.ClinicianID); err != nil {
		return err
	}
	return l.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                            input.OrgID,
		ClinicianID:                      input.ClinicianID,
		EntryID:                          input.EntryID,
		StatDate:                         input.OccurredAt,
		CareRelationshipEstablishedCount: 1,
	})
}

func (l episodeLifecycler) applyCareRelationshipTransferred(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := l.projection.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventCareRelationshipTransferred, "testee", input.TesteeID, "clinician", input.ClinicianID); err != nil {
		return err
	}
	return l.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                            input.OrgID,
		ClinicianID:                      input.ClinicianID,
		StatDate:                         input.OccurredAt,
		CareRelationshipTransferredCount: 1,
	})
}

func (l episodeLifecycler) applyAnswerSheetSubmitted(ctx context.Context, input BehaviorProjectEventInput) error {
	if err := l.projection.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventAnswerSheetSubmitted, "answersheet", input.AnswerSheetID, "testee", input.TesteeID); err != nil {
		return err
	}
	episode, err := l.repo.FindEpisodeByAnswerSheetID(ctx, input.OrgID, input.AnswerSheetID)
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
		if latestIntake, err := l.repo.FindLatestFootprintByEvent(ctx, input.OrgID, input.TesteeID, domainStatistics.BehaviorEventIntakeConfirmed, input.OccurredAt, behaviorAttributionWindow); err != nil {
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
		if err := l.repo.SaveEpisode(ctx, episode); err != nil {
			return err
		}
	}
	return l.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                     input.OrgID,
		ClinicianID:               valueOrZero(episode.ClinicianID),
		EntryID:                   valueOrZero(episode.EntryID),
		StatDate:                  input.OccurredAt,
		AnswerSheetSubmittedCount: 1,
	})
}

func (l episodeLifecycler) applyAssessmentCreated(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventStatus, error) {
	if err := l.projection.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventAssessmentCreated, "assessment", input.AssessmentID, "testee", input.TesteeID); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	episode, err := l.repo.FindEpisodeByAnswerSheetID(ctx, input.OrgID, input.AnswerSheetID)
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
	if err := l.repo.SaveEpisode(ctx, episode); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	return BehaviorProjectEventStatusCompleted, nil
}

func (l episodeLifecycler) applyReportGenerated(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventStatus, error) {
	if err := l.projection.appendBehaviorFootprint(ctx, input, domainStatistics.BehaviorEventReportGenerated, "report", input.ReportID, "assessment", input.AssessmentID); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	episode, err := l.repo.FindEpisodeByAssessmentID(ctx, input.OrgID, input.AssessmentID)
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
	if err := l.repo.SaveEpisode(ctx, episode); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	if err := l.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                  input.OrgID,
		ClinicianID:            valueOrZero(episode.ClinicianID),
		EntryID:                valueOrZero(episode.EntryID),
		StatDate:               input.OccurredAt,
		AssessmentCreatedCount: 1,
		ReportGeneratedCount:   1,
		EpisodeCompletedCount:  1,
	}); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	return BehaviorProjectEventStatusCompleted, nil
}

func (l episodeLifecycler) applyAssessmentFailed(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventStatus, error) {
	episode, err := l.repo.FindEpisodeByAssessmentID(ctx, input.OrgID, input.AssessmentID)
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
	if err := l.repo.SaveEpisode(ctx, episode); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	if err := l.repo.ApplyAnalyticsProjectionMutation(ctx, domainStatistics.AnalyticsProjectionMutation{
		OrgID:                 input.OrgID,
		ClinicianID:           valueOrZero(episode.ClinicianID),
		EntryID:               valueOrZero(episode.EntryID),
		StatDate:              input.OccurredAt,
		EpisodeFailedCount:    1,
		AssessmentFailedCount: 1,
	}); err != nil {
		return BehaviorProjectEventStatusCompleted, err
	}
	return BehaviorProjectEventStatusCompleted, nil
}

func (l episodeLifecycler) rebindEpisodesToIntake(ctx context.Context, input BehaviorProjectEventInput) error {
	episodes, err := l.repo.ListEpisodesForAttribution(ctx, input.OrgID, input.TesteeID, input.OccurredAt, behaviorAttributionWindow)
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
		if err := l.repo.SaveEpisode(ctx, episode); err != nil {
			return err
		}
		if err := l.projection.reallocateEpisodeProjection(ctx, episode, oldClinicianID, oldEntryID, input.ClinicianID, input.EntryID); err != nil {
			return err
		}
	}
	return nil
}
