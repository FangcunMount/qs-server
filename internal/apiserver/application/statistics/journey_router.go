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
	case domainStatistics.EventTypeFootprintEntryOpened:
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyEntryOpened(ctx, input)
	case domainStatistics.EventTypeFootprintIntakeConfirmed:
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyIntakeConfirmed(ctx, input)
	case domainStatistics.EventTypeFootprintTesteeProfileCreated:
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyTesteeProfileCreated(ctx, input)
	case domainStatistics.EventTypeFootprintCareRelationshipEstablished:
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyCareRelationshipEstablished(ctx, input)
	case domainStatistics.EventTypeFootprintCareRelationshipTransferred:
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyCareRelationshipTransferred(ctx, input)
	case domainStatistics.EventTypeFootprintAnswerSheetSubmitted:
		return BehaviorProjectEventStatusCompleted, r.lifecycler.applyAnswerSheetSubmitted(ctx, input)
	case domainStatistics.EventTypeFootprintAssessmentCreated:
		return r.lifecycler.applyAssessmentCreated(ctx, input)
	case domainStatistics.EventTypeFootprintReportGenerated:
		return r.lifecycler.applyReportGenerated(ctx, input)
	case domainAssessment.EventTypeFailed:
		return r.lifecycler.applyAssessmentFailed(ctx, input)
	default:
		return BehaviorProjectEventStatusCompleted, fmt.Errorf("unsupported behavior event type %q", input.EventType)
	}
}
