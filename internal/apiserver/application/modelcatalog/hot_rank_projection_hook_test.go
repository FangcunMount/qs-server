package modelcatalog

import (
	"context"
	"testing"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type hotRankProjectionCapture struct{ facts []hotrank.SubmissionFact }

func (p *hotRankProjectionCapture) ProjectSubmission(_ context.Context, fact hotrank.SubmissionFact) error {
	p.facts = append(p.facts, fact)
	return nil
}

func TestCatalogHotRankProjectionHookProjectsSubmittedAnswerSheet(t *testing.T) {
	t.Parallel()
	submittedAt := time.Date(2026, 5, 1, 9, 30, 0, 0, time.UTC)
	evt := event.New(domainAnswerSheet.EventTypeSubmitted, domainAnswerSheet.AggregateType, "sheet-1", domainAnswerSheet.AnswerSheetSubmittedData{AnswerSheetID: "sheet-1", QuestionnaireCode: "QNR-1", SubmittedAt: submittedAt})
	projection := &hotRankProjectionCapture{}
	hook := NewCatalogHotRankProjectionHook(projection)
	if err := hook.BeforePublish(context.Background(), appEventing.PendingOutboxEvent{EventID: "evt-1", Event: evt}); err != nil {
		t.Fatalf("BeforePublish() error = %v", err)
	}
	if len(projection.facts) != 1 || projection.facts[0].EventID != "evt-1" || projection.facts[0].QuestionnaireCode != "QNR-1" || !projection.facts[0].SubmittedAt.Equal(submittedAt) {
		t.Fatalf("projected facts = %#v", projection.facts)
	}
}

func TestCatalogHotRankProjectionHookDecodesStoredOutboxEvent(t *testing.T) {
	t.Parallel()
	evt := event.New(domainAnswerSheet.EventTypeSubmitted, domainAnswerSheet.AggregateType, "sheet-1", domainAnswerSheet.AnswerSheetSubmittedData{AnswerSheetID: "sheet-1", QuestionnaireCode: "QNR-1"})
	payload, err := eventcodec.EncodeDomainEvent(evt)
	if err != nil {
		t.Fatalf("EncodeDomainEvent: %v", err)
	}
	decoded, err := eventcodec.DecodeDomainEvent(payload)
	if err != nil {
		t.Fatalf("DecodeDomainEvent: %v", err)
	}
	projection := &hotRankProjectionCapture{}
	if err := NewCatalogHotRankProjectionHook(projection).BeforePublish(context.Background(), appEventing.PendingOutboxEvent{EventID: "evt-1", Event: decoded}); err != nil {
		t.Fatalf("BeforePublish() error = %v", err)
	}
	if len(projection.facts) != 1 || projection.facts[0].QuestionnaireCode != "QNR-1" {
		t.Fatalf("projected facts = %#v", projection.facts)
	}
}
