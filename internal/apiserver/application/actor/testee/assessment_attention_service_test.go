package testee

import (
	"context"
	"testing"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func TestSyncAssessmentAttentionMarksHighRiskWithoutChangingTags(t *testing.T) {
	item := domain.NewTestee(1, "testee", domain.GenderUnknown, nil)
	item.SetID(domain.ID(10))
	item.SetTags([]domain.Tag{domain.Tag("risk_high"), domain.Tag("manual")})
	repo := &assessmentAttentionRepoStub{item: item}
	var txCalls int
	service := NewAssessmentAttentionService(
		repo,
		domain.NewEditor(domain.NewValidator(repo)),
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(txCtx context.Context) error) error {
			txCalls++
			return fn(ctx)
		}),
	)

	result, err := service.SyncAssessmentAttention(context.Background(), 10, "severe", true)
	if err != nil {
		t.Fatalf("SyncAssessmentAttention returned error: %v", err)
	}

	if txCalls != 1 {
		t.Fatalf("expected one transaction, got %d", txCalls)
	}
	if repo.findByIDCalls != 1 || repo.updateCalls != 1 {
		t.Fatalf("expected one aggregate load and update, got find=%d update=%d", repo.findByIDCalls, repo.updateCalls)
	}
	if !result.KeyFocusMarked || !item.IsKeyFocus() {
		t.Fatalf("expected high risk result to mark key focus")
	}
	assertStrings(t, item.TagsAsStrings(), []string{"risk_high", "manual"})
}

func TestSyncAssessmentAttentionDoesNotUnmarkOrRewriteTagsForLowerRisk(t *testing.T) {
	item := domain.NewTestee(1, "testee", domain.GenderUnknown, nil)
	item.SetID(domain.ID(11))
	item.SetTags([]domain.Tag{domain.Tag("risk_high"), domain.Tag("risk_severe"), domain.Tag("manual")})
	item.SetKeyFocus(true)
	repo := &assessmentAttentionRepoStub{item: item}
	var txCalls int
	service := NewAssessmentAttentionService(
		repo,
		domain.NewEditor(domain.NewValidator(repo)),
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(txCtx context.Context) error) error {
			txCalls++
			return fn(ctx)
		}),
	)

	result, err := service.SyncAssessmentAttention(context.Background(), 11, "low", false)
	if err != nil {
		t.Fatalf("SyncAssessmentAttention returned error: %v", err)
	}

	if result.KeyFocusMarked {
		t.Fatalf("expected lower risk sync not to mark key focus")
	}
	if !item.IsKeyFocus() {
		t.Fatalf("expected existing key focus marker to be preserved")
	}
	assertStrings(t, item.TagsAsStrings(), []string{"risk_high", "risk_severe", "manual"})
	if txCalls != 0 || repo.findByIDCalls != 0 || repo.updateCalls != 0 {
		t.Fatalf("expected lower risk sync to be a no-op, got tx=%d find=%d update=%d", txCalls, repo.findByIDCalls, repo.updateCalls)
	}
}

func TestSyncAssessmentAttentionDoesNothingWhenCallerDoesNotRequestKeyFocus(t *testing.T) {
	item := domain.NewTestee(1, "testee", domain.GenderUnknown, nil)
	item.SetID(domain.ID(12))
	repo := &assessmentAttentionRepoStub{item: item}
	service := NewAssessmentAttentionService(
		repo,
		domain.NewEditor(domain.NewValidator(repo)),
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(txCtx context.Context) error) error {
			return fn(ctx)
		}),
	)

	result, err := service.SyncAssessmentAttention(context.Background(), 12, "high", false)
	if err != nil {
		t.Fatalf("SyncAssessmentAttention returned error: %v", err)
	}

	if result.KeyFocusMarked || item.IsKeyFocus() {
		t.Fatalf("expected high risk without mark_key_focus to remain unmarked")
	}
	if repo.findByIDCalls != 0 || repo.updateCalls != 0 {
		t.Fatalf("expected no repository calls, got find=%d update=%d", repo.findByIDCalls, repo.updateCalls)
	}
}

type assessmentAttentionRepoStub struct {
	item          *domain.Testee
	findByIDCalls int
	updateCalls   int
}

func (s *assessmentAttentionRepoStub) Save(context.Context, *domain.Testee) error { return nil }

func (s *assessmentAttentionRepoStub) Update(_ context.Context, item *domain.Testee) error {
	s.updateCalls++
	s.item = item
	return nil
}

func (s *assessmentAttentionRepoStub) FindByID(_ context.Context, id domain.ID) (*domain.Testee, error) {
	s.findByIDCalls++
	return s.item, nil
}

func (s *assessmentAttentionRepoStub) FindByProfile(context.Context, int64, uint64) (*domain.Testee, error) {
	return nil, nil
}

func (s *assessmentAttentionRepoStub) Delete(context.Context, domain.ID) error { return nil }

func assertStrings(t *testing.T, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, actual)
		}
	}
}
