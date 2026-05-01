package scale

import (
	"context"
	"testing"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type scaleHotRankProjectionCapture struct {
	facts []domainScale.ScaleHotRankSubmissionFact
	err   error
}

func (p *scaleHotRankProjectionCapture) ProjectSubmission(_ context.Context, fact domainScale.ScaleHotRankSubmissionFact) error {
	p.facts = append(p.facts, fact)
	return p.err
}

func TestScaleHotRankProjectionHookProjectsAnswerSheetSubmittedEvent(t *testing.T) {
	submittedAt := time.Date(2026, 5, 1, 9, 30, 0, 0, time.UTC)
	evt := event.New(domainAnswerSheet.EventTypeSubmitted, domainAnswerSheet.AggregateType, "sheet-1", domainAnswerSheet.AnswerSheetSubmittedData{
		AnswerSheetID:     "sheet-1",
		QuestionnaireCode: "QNR-1",
		SubmittedAt:       submittedAt,
	})
	projection := &scaleHotRankProjectionCapture{}
	hook := NewScaleHotRankProjectionHook(projection)

	if err := hook.BeforePublish(context.Background(), appEventing.PendingOutboxEvent{EventID: "evt-1", Event: evt}); err != nil {
		t.Fatalf("BeforePublish() error = %v", err)
	}
	if len(projection.facts) != 1 {
		t.Fatalf("projected facts = %#v, want one", projection.facts)
	}
	fact := projection.facts[0]
	if fact.EventID != "evt-1" || fact.QuestionnaireCode != "QNR-1" || !fact.SubmittedAt.Equal(submittedAt) {
		t.Fatalf("projected fact = %+v, want event id/questionnaire/submitted_at", fact)
	}
}

func TestScaleHotRankProjectionHookDecodesStoredOutboxEvent(t *testing.T) {
	submittedAt := time.Date(2026, 5, 1, 9, 30, 0, 0, time.UTC)
	evt := event.New(domainAnswerSheet.EventTypeSubmitted, domainAnswerSheet.AggregateType, "sheet-1", domainAnswerSheet.AnswerSheetSubmittedData{
		AnswerSheetID:     "sheet-1",
		QuestionnaireCode: "QNR-1",
		SubmittedAt:       submittedAt,
	})
	payload, err := eventcodec.EncodeDomainEvent(evt)
	if err != nil {
		t.Fatalf("EncodeDomainEvent() error = %v", err)
	}
	decoded, err := eventcodec.DecodeDomainEvent(payload)
	if err != nil {
		t.Fatalf("DecodeDomainEvent() error = %v", err)
	}

	projection := &scaleHotRankProjectionCapture{}
	hook := NewScaleHotRankProjectionHook(projection)
	if err := hook.BeforePublish(context.Background(), appEventing.PendingOutboxEvent{EventID: "evt-1", Event: decoded}); err != nil {
		t.Fatalf("BeforePublish() error = %v", err)
	}
	if len(projection.facts) != 1 || projection.facts[0].QuestionnaireCode != "QNR-1" {
		t.Fatalf("projected facts = %#v, want decoded questionnaire code", projection.facts)
	}
}

func TestScaleHotRankProjectionHookIgnoresOtherEvents(t *testing.T) {
	projection := &scaleHotRankProjectionCapture{}
	hook := NewScaleHotRankProjectionHook(projection)
	evt := event.New("assessment.submitted", "Assessment", "assessment-1", struct{}{})

	if err := hook.BeforePublish(context.Background(), appEventing.PendingOutboxEvent{EventID: "evt-1", Event: evt}); err != nil {
		t.Fatalf("BeforePublish() error = %v", err)
	}
	if len(projection.facts) != 0 {
		t.Fatalf("projected facts = %#v, want none", projection.facts)
	}
}
