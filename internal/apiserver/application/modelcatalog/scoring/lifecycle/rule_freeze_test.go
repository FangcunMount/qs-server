package lifecycle

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func TestPublishedScaleFreezesRuleMutationsButAllowsDisplayUpdate(t *testing.T) {
	t.Parallel()

	model := newApplicationScaleForFreezeTest(t, domain.ModelStatusPublished)
	modelRepo := &authoringModelRepoStub{model: model}

	factorSvc := factor.NewService(modelRepo, nil, &scaleEventPublisherStub{})
	if _, err := factorSvc.AddFactor(context.Background(), shared.AddFactorDTO{
		ScaleCode:     model.Code,
		Code:          "F2",
		Title:         "Factor 2",
		QuestionCodes: []string{"Q2"},
	}); err != nil {
		t.Fatalf("AddFactor() error = %v, want nil after draft fork", err)
	}
	if modelRepo.updateCount != 2 || !model.IsDraft() {
		t.Fatalf("published edit fork = updates:%d status:%s, want 2 updates and draft status",
			modelRepo.updateCount, model.Status)
	}

	modelRepo.updateCount = 0
	lifecycleSvc := newAuthoringLifecycleService(nil, modelRepo, nil)
	if _, err := lifecycleSvc.UpdateQuestionnaire(context.Background(), shared.UpdateScaleQuestionnaireDTO{
		Code:                 model.Code,
		QuestionnaireCode:    "Q2",
		QuestionnaireVersion: "2.0",
	}); err == nil {
		t.Fatal("UpdateQuestionnaire() error = nil, want published rule freeze error")
	}
	if modelRepo.updateCount != 0 {
		t.Fatalf("questionnaire mutation updated repo %d times, want 0", modelRepo.updateCount)
	}

	if _, err := lifecycleSvc.UpdateBasicInfo(context.Background(), shared.UpdateScaleBasicInfoDTO{
		Code:  model.Code,
		Title: "Updated Scale",
	}); err != nil {
		t.Fatalf("UpdateBasicInfo() error = %v, want nil", err)
	}
	if modelRepo.updateCount != 1 {
		t.Fatalf("display update repo updates = %d, want 1", modelRepo.updateCount)
	}
}

func TestQuestionnaireBindingSyncerSyncsDraftOnly(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		status     domain.ModelStatus
		wantUpdate bool
	}{
		{name: "draft", status: domain.ModelStatusDraft, wantUpdate: true},
		{name: "published", status: domain.ModelStatusPublished, wantUpdate: false},
		{name: "archived", status: domain.ModelStatusArchived, wantUpdate: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model := newApplicationScaleForFreezeTest(t, domain.ModelStatusDraft)
			model.Status = tc.status
			modelRepo := &authoringModelRepoStub{model: model}
			syncer := NewQuestionnaireBindingSyncer(modelRepo)

			if err := syncer.SyncQuestionnaireVersion(context.Background(), "Q1", "2.0"); err != nil {
				t.Fatalf("SyncQuestionnaireVersion() error = %v", err)
			}
			if got := modelRepo.updateCount > 0; got != tc.wantUpdate {
				t.Fatalf("updated = %v, want %v", got, tc.wantUpdate)
			}
			if tc.wantUpdate && model.Binding.QuestionnaireVersion != "2.0" {
				t.Fatalf("questionnaire version = %q, want 2.0", model.Binding.QuestionnaireVersion)
			}
		})
	}
}

func newApplicationScaleForFreezeTest(t *testing.T, status domain.ModelStatus) *domain.AssessmentModel {
	t.Helper()
	return newLifecycleScaleAssessmentModel(
		t,
		"S1",
		"Scale 1",
		"Q1",
		"1.0",
		status,
		lifecycleDefaultFactorSnapshots(),
	)
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
