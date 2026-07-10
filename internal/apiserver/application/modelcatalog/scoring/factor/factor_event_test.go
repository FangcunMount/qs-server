package factor

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type scaleEventPublisherStub struct {
	events []event.DomainEvent
}

func (p *scaleEventPublisherStub) Publish(_ context.Context, evt event.DomainEvent) error {
	p.events = append(p.events, evt)
	return nil
}

func (p *scaleEventPublisherStub) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func TestFactorMutationPublishesCollectedDomainEventOnce(t *testing.T) {
	t.Parallel()

	model := newDraftAssessmentModelForFactorTest(t)
	modelRepo := &factorAssessmentModelRepoStub{model: model}
	publisher := &scaleEventPublisherStub{}
	factorSvc := NewService(modelRepo, nil, publisher)

	if _, err := factorSvc.AddFactor(context.Background(), shared.AddFactorDTO{
		ScaleCode:     model.Code,
		Code:          "F2",
		Title:         "Factor 2",
		QuestionCodes: []string{"Q2"},
	}); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published event count = %d, want 1", len(publisher.events))
	}
	if publisher.events[0].EventType() != eventcatalog.ScaleChanged {
		t.Fatalf("event type = %q, want %s", publisher.events[0].EventType(), eventcatalog.ScaleChanged)
	}
}
