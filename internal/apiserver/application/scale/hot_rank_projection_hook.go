package scale

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type answerSheetSubmittedPayload interface {
	Payload() domainAnswerSheet.AnswerSheetSubmittedData
}

type scaleHotRankProjectionHook struct {
	projection domainScale.ScaleHotRankProjection
}

func NewScaleHotRankProjectionHook(projection domainScale.ScaleHotRankProjection) appEventing.OutboxBeforePublishHook {
	if projection == nil {
		return nil
	}
	return scaleHotRankProjectionHook{projection: projection}
}

func (h scaleHotRankProjectionHook) BeforePublish(ctx context.Context, pending appEventing.PendingOutboxEvent) error {
	if h.projection == nil || pending.Event == nil || pending.Event.EventType() != domainAnswerSheet.EventTypeSubmitted {
		return nil
	}

	data, err := answerSheetSubmittedDataFromEvent(pending.Event)
	if err != nil {
		return err
	}
	eventID := strings.TrimSpace(pending.EventID)
	if eventID == "" {
		eventID = pending.Event.EventID()
	}
	submittedAt := data.SubmittedAt
	if submittedAt.IsZero() {
		submittedAt = pending.Event.OccurredAt()
	}
	return h.projection.ProjectSubmission(ctx, domainScale.ScaleHotRankSubmissionFact{
		EventID:           eventID,
		QuestionnaireCode: data.QuestionnaireCode,
		SubmittedAt:       submittedAt,
	})
}

func answerSheetSubmittedDataFromEvent(evt event.DomainEvent) (domainAnswerSheet.AnswerSheetSubmittedData, error) {
	if typed, ok := evt.(answerSheetSubmittedPayload); ok {
		return typed.Payload(), nil
	}

	payload, err := eventcodec.EncodeDomainEvent(evt)
	if err != nil {
		return domainAnswerSheet.AnswerSheetSubmittedData{}, err
	}
	env, err := eventcodec.DecodeEnvelope(payload)
	if err != nil {
		return domainAnswerSheet.AnswerSheetSubmittedData{}, err
	}
	var data domainAnswerSheet.AnswerSheetSubmittedData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return domainAnswerSheet.AnswerSheetSubmittedData{}, fmt.Errorf("decode answersheet submitted payload: %w", err)
	}
	return data, nil
}
