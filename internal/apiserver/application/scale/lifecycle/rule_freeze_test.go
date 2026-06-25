package lifecycle

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/authoring/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type ruleFreezeScaleRepoStub struct {
	item          *domainScale.MedicalScale
	updateCount   int
	snapshotCount int
	clearedActive bool
}

func (r *ruleFreezeScaleRepoStub) Create(context.Context, *domainScale.MedicalScale) error {
	return nil
}

func (r *ruleFreezeScaleRepoStub) CreatePublishedSnapshot(context.Context, *domainScale.MedicalScale, bool) error {
	r.snapshotCount++
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

func (r *ruleFreezeScaleRepoStub) FindPublishedByCode(context.Context, string) (*domainScale.MedicalScale, error) {
	return r.FindByCode(context.Background(), "")
}

func (r *ruleFreezeScaleRepoStub) FindByQuestionnaireCode(context.Context, string) (*domainScale.MedicalScale, error) {
	if r.item == nil {
		return nil, domainScale.ErrNotFound
	}
	return r.item, nil
}

func (r *ruleFreezeScaleRepoStub) FindPublishedByQuestionnaireCode(context.Context, string) (*domainScale.MedicalScale, error) {
	return r.FindByQuestionnaireCode(context.Background(), "")
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

func (r *ruleFreezeScaleRepoStub) SetActivePublishedVersion(context.Context, string, string) error {
	return nil
}

func (r *ruleFreezeScaleRepoStub) ClearActivePublishedVersion(context.Context, string) error {
	r.clearedActive = true
	return nil
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

func TestPublishedScaleFreezesRuleMutationsButAllowsDisplayUpdate(t *testing.T) {
	t.Parallel()

	published := newApplicationScaleForFreezeTest(t, domainScale.StatusPublished)
	repo := &ruleFreezeScaleRepoStub{item: published}

	factorSvc := factor.NewService(repo, nil, &scaleEventPublisherStub{})
	if _, err := factorSvc.AddFactor(context.Background(), shared.AddFactorDTO{
		ScaleCode:     published.GetCode().String(),
		Code:          "F2",
		Title:         "Factor 2",
		QuestionCodes: []string{"Q2"},
	}); err != nil {
		t.Fatalf("AddFactor() error = %v, want nil after draft fork", err)
	}
	if repo.snapshotCount != 1 || repo.updateCount != 1 || !repo.item.IsDraft() || repo.item.GetScaleVersion() != "1.0.1" {
		t.Fatalf("published edit fork = snapshots:%d updates:%d status:%s version:%s, want snapshot/update draft 1.0.1",
			repo.snapshotCount, repo.updateCount, repo.item.GetStatus(), repo.item.GetScaleVersion())
	}

	repo.updateCount = 0
	lifecycleSvc := &lifecycleService{repo: repo, baseInfo: domainScale.BaseInfo{}, eventPublisher: &scaleEventPublisherStub{}}
	if _, err := lifecycleSvc.UpdateQuestionnaire(context.Background(), shared.UpdateScaleQuestionnaireDTO{
		Code:                 published.GetCode().String(),
		QuestionnaireCode:    "Q2",
		QuestionnaireVersion: "2.0",
	}); err == nil {
		t.Fatal("UpdateQuestionnaire() error = nil, want published rule freeze error")
	}
	if repo.updateCount != 0 {
		t.Fatalf("questionnaire mutation updated repo %d times, want 0", repo.updateCount)
	}

	if _, err := lifecycleSvc.UpdateBasicInfo(context.Background(), shared.UpdateScaleBasicInfoDTO{
		Code:  published.GetCode().String(),
		Title: "Updated Scale",
	}); err != nil {
		t.Fatalf("UpdateBasicInfo() error = %v, want nil", err)
	}
	if repo.updateCount != 1 {
		t.Fatalf("display update repo updates = %d, want 1", repo.updateCount)
	}
}

func TestQuestionnaireBindingSyncerSyncsDraftOnly(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		status     domainScale.Status
		wantUpdate bool
	}{
		{name: "draft", status: domainScale.StatusDraft, wantUpdate: true},
		{name: "published", status: domainScale.StatusPublished, wantUpdate: false},
		{name: "archived", status: domainScale.StatusArchived, wantUpdate: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			item := newApplicationScaleForFreezeTest(t, tc.status)
			repo := &ruleFreezeScaleRepoStub{item: item}
			syncer := NewQuestionnaireBindingSyncer(repo)

			if err := syncer.SyncQuestionnaireVersion(context.Background(), "Q1", "2.0"); err != nil {
				t.Fatalf("SyncQuestionnaireVersion() error = %v", err)
			}
			if got := repo.updateCount > 0; got != tc.wantUpdate {
				t.Fatalf("updated = %v, want %v", got, tc.wantUpdate)
			}
			if tc.wantUpdate && repo.item.GetQuestionnaireVersion() != "2.0" {
				t.Fatalf("questionnaire version = %q, want 2.0", repo.item.GetQuestionnaireVersion())
			}
		})
	}
}

func newApplicationScaleForFreezeTest(t *testing.T, status domainScale.Status) *domainScale.MedicalScale {
	t.Helper()

	factor, err := domainScale.NewFactor(
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
		domainScale.WithStatus(status),
		domainScale.WithFactors([]*domainScale.Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return item
}
