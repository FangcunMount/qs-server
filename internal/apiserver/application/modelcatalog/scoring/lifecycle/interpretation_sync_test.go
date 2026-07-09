package lifecycle

import (
	"context"
	stderrors "errors"
	"testing"

	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type interpretationPublisherStub struct {
	calls int
}

func (s *interpretationPublisherStub) PublishPublishedScale(context.Context, *scaledefinition.MedicalScale) error {
	s.calls++
	return nil
}

type assessmentSnapshotPublisherStub struct {
	calls int
	err   error
}

func (s *assessmentSnapshotPublisherStub) PublishAssessmentSnapshot(context.Context, *scaledefinition.MedicalScale) error {
	s.calls++
	return s.err
}

func TestPublishUsesAssessmentSnapshotPublisherWhenConfigured(t *testing.T) {
	scale := newPublishableScaleForTest(t)
	repo := &scalePublishRepoStub{scale: scale}
	catalog := publishedQuestionnaireCatalogForScalePublish()
	legacyPublisher := &interpretationPublisherStub{}
	snapshotPublisher := &assessmentSnapshotPublisherStub{}
	svc := NewService(
		repo,
		catalog,
		event.NewNopEventPublisher(),
		nil,
		WithScalePublisher(legacyPublisher),
		WithAssessmentSnapshotPublisher(snapshotPublisher),
	)
	if _, err := svc.Publish(context.Background(), "SCL-001"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if snapshotPublisher.calls != 1 {
		t.Fatalf("assessment snapshot publish calls = %d, want 1", snapshotPublisher.calls)
	}
	if legacyPublisher.calls != 0 {
		t.Fatalf("legacy interpretation sync calls = %d, want 0", legacyPublisher.calls)
	}
	wantRepoCalls := []string{"update", "create_published_snapshot", "set_active_published_version"}
	if got := repo.calls; !equalStringSlices(got, wantRepoCalls) {
		t.Fatalf("repo calls = %v, want %v", got, wantRepoCalls)
	}
}

func TestPublishFallsBackToLegacyScalePublisher(t *testing.T) {
	scale := newPublishableScaleForTest(t)
	repo := &scalePublishRepoStub{scale: scale}
	catalog := publishedQuestionnaireCatalogForScalePublish()
	publisher := &interpretationPublisherStub{}
	svc := NewService(
		repo,
		catalog,
		event.NewNopEventPublisher(),
		nil,
		WithScalePublisher(publisher),
	)
	if _, err := svc.Publish(context.Background(), "SCL-001"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if publisher.calls != 1 {
		t.Fatalf("interpretation sync calls = %d, want 1", publisher.calls)
	}
}

func TestPublishDoesNotEmitEventsOrCacheSignalWhenSnapshotPublishFails(t *testing.T) {
	scale := newPublishableScaleForTest(t)
	repo := &scalePublishRepoStub{scale: scale}
	eventPublisher := &scaleEventPublisherStub{}
	cacheNotifier := &cacheSignalNotifierStub{}
	publishErr := stderrors.New("snapshot publish failed")
	svc := NewService(
		repo,
		publishedQuestionnaireCatalogForScalePublish(),
		eventPublisher,
		nil,
		WithCacheSignalNotifier(cacheNotifier),
		WithAssessmentSnapshotPublisher(&assessmentSnapshotPublisherStub{err: publishErr}),
	)
	if _, err := svc.Publish(context.Background(), "SCL-001"); err == nil {
		t.Fatal("Publish error = nil, want snapshot publish error")
	}
	wantRepoCalls := []string{"update", "create_published_snapshot", "set_active_published_version"}
	if got := repo.calls; !equalStringSlices(got, wantRepoCalls) {
		t.Fatalf("repo calls = %v, want %v", got, wantRepoCalls)
	}
	if len(eventPublisher.events) != 0 {
		t.Fatalf("events = %v, want none", eventPublisher.events)
	}
	if cacheNotifier.calls != 0 {
		t.Fatalf("cache signal calls = %d, want 0", cacheNotifier.calls)
	}
}

type scalePublishRepoStub struct {
	scale *scaledefinition.MedicalScale
	calls []string
}

func (r *scalePublishRepoStub) Create(context.Context, *scaledefinition.MedicalScale) error {
	return nil
}
func (r *scalePublishRepoStub) CreatePublishedSnapshot(context.Context, *scaledefinition.MedicalScale, bool) error {
	r.calls = append(r.calls, "create_published_snapshot")
	return nil
}
func (r *scalePublishRepoStub) FindByCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	return r.scale, nil
}
func (r *scalePublishRepoStub) FindByCodeVersion(context.Context, string, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}
func (r *scalePublishRepoStub) FindPublishedByCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}
func (r *scalePublishRepoStub) FindByQuestionnaireCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}
func (r *scalePublishRepoStub) FindPublishedByQuestionnaireCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}
func (r *scalePublishRepoStub) FindByQuestionnaireRef(context.Context, string, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}
func (r *scalePublishRepoStub) Update(_ context.Context, scale *scaledefinition.MedicalScale) error {
	r.calls = append(r.calls, "update")
	r.scale = scale
	return nil
}
func (r *scalePublishRepoStub) SetActivePublishedVersion(context.Context, string, string) error {
	r.calls = append(r.calls, "set_active_published_version")
	return nil
}
func (r *scalePublishRepoStub) ClearActivePublishedVersion(context.Context, string) error { return nil }
func (r *scalePublishRepoStub) Remove(context.Context, string) error                      { return nil }
func (r *scalePublishRepoStub) ExistsByCode(context.Context, string) (bool, error)        { return true, nil }

func newPublishableScaleForTest(t *testing.T) *scaledefinition.MedicalScale {
	t.Helper()
	factor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("total"),
		"总分",
		scaledefinition.WithIsTotalScore(true),
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}),
		scaledefinition.WithScoringStrategy(scaledefinition.ScoringStrategySum),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(scaledefinition.NewScoreRange(0, 10), scaledefinition.RiskLevelLow, "low", "watch"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor: %v", err)
	}
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCL-001"),
		"Demo",
		scaledefinition.WithQuestionnaire(meta.NewCode("QNR-001"), "1.0.0"),
		scaledefinition.WithScaleVersion("1.0.0"),
		scaledefinition.WithFactors([]*scaledefinition.Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale: %v", err)
	}
	return scale
}

type cacheSignalNotifierStub struct {
	calls int
}

func (s *cacheSignalNotifierStub) NotifyScaleCacheChanged(context.Context, string, string) {
	s.calls++
}

func publishedQuestionnaireCatalogForScalePublish() *questionnaireCatalogBindingStub {
	return &questionnaireCatalogBindingStub{
		byCode: map[string]*questionnairecatalog.Item{
			"QNR-001": {Code: "QNR-001", Version: "1.0.0", Status: "published", Type: "MedicalScale"},
		},
		byVersion: map[string]*questionnairecatalog.Item{
			"QNR-001:1.0.0": {Code: "QNR-001", Version: "1.0.0", Status: "published", Type: "MedicalScale"},
		},
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
