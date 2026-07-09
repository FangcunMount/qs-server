package factor

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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

func newDraftScaleForEventTest(t *testing.T) *scaledefinition.MedicalScale {
	t.Helper()
	f, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("F1"),
		"Factor 1",
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(scaledefinition.NewScoreRange(0, 10), scaledefinition.RiskLevelLow, "low", "watch"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor() error = %v", err)
	}
	item, err := scaledefinition.NewMedicalScale(
		meta.NewCode("S1"),
		"Scale 1",
		scaledefinition.WithID(meta.FromUint64(101)),
		scaledefinition.WithQuestionnaire(meta.NewCode("Q1"), "1.0"),
		scaledefinition.WithStatus(scaledefinition.StatusDraft),
		scaledefinition.WithFactors([]*scaledefinition.Factor{f}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return item
}

func TestFactorMutationPublishesCollectedDomainEventOnce(t *testing.T) {
	t.Parallel()

	item := newDraftScaleForEventTest(t)
	model, err := legacyadapter.AssessmentModelFromMedicalScale(item, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale() error = %v", err)
	}
	model.Code = item.GetCode().String()
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
	if publisher.events[0].EventType() != scaledefinition.EventTypeChanged {
		t.Fatalf("event type = %q, want %s", publisher.events[0].EventType(), scaledefinition.EventTypeChanged)
	}
	if len(item.Events()) != 0 {
		t.Fatalf("domain events were not cleared after publish")
	}
}
