package lifecycle

import (
	"context"
	"testing"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type interpretationPublisherStub struct {
	calls int
}

func (s *interpretationPublisherStub) PublishPublishedScale(context.Context, *domainScale.MedicalScale) error {
	s.calls++
	return nil
}

func TestPublishSyncsInterpretationRules(t *testing.T) {
	scale := newPublishableScaleForTest(t)
	repo := &scalePublishRepoStub{scale: scale}
	catalog := &questionnaireCatalogBindingStub{
		byCode: map[string]*questionnairecatalog.Item{
			"QNR-001": {Code: "QNR-001", Version: "1.0.0", Status: "published", Type: "MedicalScale"},
		},
		byVersion: map[string]*questionnairecatalog.Item{
			"QNR-001:1.0.0": {Code: "QNR-001", Version: "1.0.0", Status: "published", Type: "MedicalScale"},
		},
	}
	publisher := &interpretationPublisherStub{}
	svc := NewService(
		repo,
		catalog,
		event.NewNopEventPublisher(),
		nil,
		WithRuleSetPublisher(publisher),
	)
	if _, err := svc.Publish(context.Background(), "SCL-001"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if publisher.calls != 1 {
		t.Fatalf("interpretation sync calls = %d, want 1", publisher.calls)
	}
}

type scalePublishRepoStub struct {
	scale *domainScale.MedicalScale
}

func (r *scalePublishRepoStub) Create(context.Context, *domainScale.MedicalScale) error { return nil }
func (r *scalePublishRepoStub) CreatePublishedSnapshot(context.Context, *domainScale.MedicalScale, bool) error {
	return nil
}
func (r *scalePublishRepoStub) FindByCode(context.Context, string) (*domainScale.MedicalScale, error) {
	return r.scale, nil
}
func (r *scalePublishRepoStub) FindByCodeVersion(context.Context, string, string) (*domainScale.MedicalScale, error) {
	return nil, domainScale.ErrNotFound
}
func (r *scalePublishRepoStub) FindPublishedByCode(context.Context, string) (*domainScale.MedicalScale, error) {
	return nil, domainScale.ErrNotFound
}
func (r *scalePublishRepoStub) FindByQuestionnaireCode(context.Context, string) (*domainScale.MedicalScale, error) {
	return nil, domainScale.ErrNotFound
}
func (r *scalePublishRepoStub) FindPublishedByQuestionnaireCode(context.Context, string) (*domainScale.MedicalScale, error) {
	return nil, domainScale.ErrNotFound
}
func (r *scalePublishRepoStub) FindByQuestionnaireRef(context.Context, string, string) (*domainScale.MedicalScale, error) {
	return nil, domainScale.ErrNotFound
}
func (r *scalePublishRepoStub) Update(_ context.Context, scale *domainScale.MedicalScale) error {
	r.scale = scale
	return nil
}
func (r *scalePublishRepoStub) SetActivePublishedVersion(context.Context, string, string) error { return nil }
func (r *scalePublishRepoStub) ClearActivePublishedVersion(context.Context, string) error       { return nil }
func (r *scalePublishRepoStub) Remove(context.Context, string) error                            { return nil }
func (r *scalePublishRepoStub) ExistsByCode(context.Context, string) (bool, error)              { return true, nil }

func newPublishableScaleForTest(t *testing.T) *domainScale.MedicalScale {
	t.Helper()
	factor, err := domainScale.NewFactor(
		domainScale.NewFactorCode("total"),
		"总分",
		domainScale.WithIsTotalScore(true),
		domainScale.WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}),
		domainScale.WithScoringStrategy(domainScale.ScoringStrategySum),
		domainScale.WithInterpretRules([]domainScale.InterpretationRule{
			domainScale.NewInterpretationRule(domainScale.NewScoreRange(0, 10), domainScale.RiskLevelLow, "low", "watch"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor: %v", err)
	}
	scale, err := domainScale.NewMedicalScale(
		meta.NewCode("SCL-001"),
		"Demo",
		domainScale.WithQuestionnaire(meta.NewCode("QNR-001"), "1.0.0"),
		domainScale.WithScaleVersion("1.0.0"),
		domainScale.WithFactors([]*domainScale.Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale: %v", err)
	}
	return scale
}
