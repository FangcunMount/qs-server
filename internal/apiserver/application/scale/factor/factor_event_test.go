package factor

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type ruleFreezeScaleRepoStub struct {
	item        *domainScale.MedicalScale
	updateCount int
}

func (r *ruleFreezeScaleRepoStub) Create(context.Context, *domainScale.MedicalScale) error {
	return nil
}

func (r *ruleFreezeScaleRepoStub) FindByCode(context.Context, string) (*domainScale.MedicalScale, error) {
	if r.item == nil {
		return nil, domainScale.ErrNotFound
	}
	return r.item, nil
}

func (r *ruleFreezeScaleRepoStub) FindByCodeVersion(context.Context, string, string) (*domainScale.MedicalScale, error) {
	return r.FindByCode(context.Background(), "")
}

func (r *ruleFreezeScaleRepoStub) FindByQuestionnaireCode(context.Context, string) (*domainScale.MedicalScale, error) {
	if r.item == nil {
		return nil, domainScale.ErrNotFound
	}
	return r.item, nil
}

func (r *ruleFreezeScaleRepoStub) FindByQuestionnaireRef(context.Context, string, string) (*domainScale.MedicalScale, error) {
	return r.FindByQuestionnaireCode(context.Background(), "")
}

func (r *ruleFreezeScaleRepoStub) Update(_ context.Context, item *domainScale.MedicalScale) error {
	r.updateCount++
	r.item = item
	return nil
}

func (r *ruleFreezeScaleRepoStub) Remove(context.Context, string) error { return nil }

func (r *ruleFreezeScaleRepoStub) ExistsByCode(context.Context, string) (bool, error) {
	return false, nil
}

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

func newDraftScaleForEventTest(t *testing.T) *domainScale.MedicalScale {
	t.Helper()
	f, err := domainScale.NewFactor(
		domainScale.NewFactorCode("F1"),
		"Factor 1",
		domainScale.WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}),
		domainScale.WithInterpretRules([]domainScale.InterpretationRule{
			domainScale.NewInterpretationRule(domainScale.NewScoreRange(0, 10), domainScale.RiskLevelLow, "low", "watch"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor() error = %v", err)
	}
	item, err := domainScale.NewMedicalScale(
		meta.NewCode("S1"),
		"Scale 1",
		domainScale.WithID(meta.FromUint64(101)),
		domainScale.WithQuestionnaire(meta.NewCode("Q1"), "1.0"),
		domainScale.WithStatus(domainScale.StatusDraft),
		domainScale.WithFactors([]*domainScale.Factor{f}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return item
}

func TestFactorMutationPublishesCollectedDomainEventOnce(t *testing.T) {
	t.Parallel()

	item := newDraftScaleForEventTest(t)
	repo := &ruleFreezeScaleRepoStub{item: item}
	publisher := &scaleEventPublisherStub{}
	factorSvc := NewService(repo, nil, publisher)

	if _, err := factorSvc.AddFactor(context.Background(), shared.AddFactorDTO{
		ScaleCode:     item.GetCode().String(),
		Code:          "F2",
		Title:         "Factor 2",
		QuestionCodes: []string{"Q2"},
	}); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published event count = %d, want 1", len(publisher.events))
	}
	if publisher.events[0].EventType() != domainScale.EventTypeChanged {
		t.Fatalf("event type = %q, want %s", publisher.events[0].EventType(), domainScale.EventTypeChanged)
	}
	if len(item.Events()) != 0 {
		t.Fatalf("domain events were not cleared after publish")
	}
}
