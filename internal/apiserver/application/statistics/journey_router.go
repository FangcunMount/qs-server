package statistics

import (
	"context"
	"fmt"

	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type behaviorEventRouter struct {
	lifecycler episodeLifecycler
}

func (r behaviorEventRouter) projectEvent(ctx context.Context, input BehaviorProjectEventInput) (BehaviorProjectEventStatus, error) {
	switch input.EventType {
	case string(domainStatistics.BehaviorEventEntryOpened):
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyEntryOpened(ctx, input)
	case string(domainStatistics.BehaviorEventIntakeConfirmed):
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyIntakeConfirmed(ctx, input)
	case string(domainStatistics.BehaviorEventTesteeProfileCreated):
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyTesteeProfileCreated(ctx, input)
	case string(domainStatistics.BehaviorEventCareRelationshipEstablished):
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyCareRelationshipEstablished(ctx, input)
	case string(domainStatistics.BehaviorEventAnswerSheetSubmitted):
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyAnswerSheetSubmitted(ctx, input)
	case string(domainStatistics.BehaviorEventAssessmentCreated):
		return r.lifecycler.applyAssessmentCreated(ctx, input)
	case string(domainStatistics.BehaviorEventReportGenerated):
		return r.lifecycler.applyReportGenerated(ctx, input)
	case domainAssessment.EventTypeFailed:
		return r.lifecycler.applyAssessmentFailed(ctx, input)
	default:
		return BehaviorProjectEventStatusCompleted, fmt.Errorf("unsupported behavior event type %q", input.EventType)
	}
}
