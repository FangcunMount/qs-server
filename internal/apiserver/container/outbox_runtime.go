package container

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/outboxready"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
)

func (c *Container) StartOutboxReadyReconcilers(ctx context.Context) {
	if c == nil {
		return
	}
	startReconciler(ctx, c.mongoOutboxReadyIndex(), c.mongoOutboxPendingLister())
	startReconciler(ctx, c.assessmentOutboxReadyIndex(), c.assessmentOutboxPendingLister())
}

func startReconciler(ctx context.Context, index *outboxready.Index, lister outboxport.PendingEventRefLister) {
	if index == nil || lister == nil {
		return
	}
	outboxready.NewReconciler(index, lister, 0).Start(ctx)
}

func (c *Container) mongoOutboxReadyIndex() *outboxready.Index {
	if c == nil || c.SurveyModule == nil || c.SurveyModule.AnswerSheet == nil {
		return nil
	}
	return c.SurveyModule.AnswerSheet.OutboxReadyIndex
}

func (c *Container) assessmentOutboxReadyIndex() *outboxready.Index {
	if c == nil || c.EvaluationModule == nil {
		return nil
	}
	return c.EvaluationModule.OutboxReadyIndex
}

func (c *Container) mongoOutboxPendingLister() outboxport.PendingEventRefLister {
	if c == nil || c.surveyScaleInfra == nil || c.surveyScaleInfra.answerSheetRepo == nil {
		return nil
	}
	return c.surveyScaleInfra.answerSheetRepo
}

func (c *Container) assessmentOutboxPendingLister() outboxport.PendingEventRefLister {
	if c == nil || c.EvaluationModule == nil {
		return nil
	}
	return c.EvaluationModule.AssessmentOutboxPendingLister
}
