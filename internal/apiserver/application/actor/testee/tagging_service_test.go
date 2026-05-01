package testee

import (
	"context"
	"testing"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func TestTagByAssessmentResultAppliesRiskPolicyInSingleRepositoryUpdate(t *testing.T) {
	item := domain.NewTestee(1, "testee", domain.GenderUnknown, nil)
	item.SetID(domain.ID(10))
	item.SetTags([]domain.Tag{domain.TagRiskHigh, domain.Tag("manual")})
	repo := &taggingRepoStub{item: item}
	var txCalls int
	service := NewTaggingService(
		repo,
		domain.NewRiskTagPolicy(),
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(txCtx context.Context) error) error {
			txCalls++
			return fn(ctx)
		}),
	)

	result, err := service.TagByAssessmentResult(context.Background(), 10, "severe", "scale-a", []string{"factor"}, true)
	if err != nil {
		t.Fatalf("TagByAssessmentResult returned error: %v", err)
	}

	if txCalls != 1 {
		t.Fatalf("expected one transaction, got %d", txCalls)
	}
	if repo.findByIDCalls != 1 || repo.updateCalls != 1 {
		t.Fatalf("expected one aggregate load and update, got find=%d update=%d", repo.findByIDCalls, repo.updateCalls)
	}
	assertStrings(t, result.TagsRemoved, []string{"risk_high"})
	assertStrings(t, result.TagsAdded, []string{"risk_high", "risk_severe"})
	if !result.KeyFocusMarked {
		t.Fatalf("expected key focus result to be marked")
	}
	assertStrings(t, item.TagsAsStrings(), []string{"manual", "risk_high", "risk_severe"})
	if !item.IsKeyFocus() {
		t.Fatalf("expected aggregate to be marked as key focus")
	}
}

func TestTagByAssessmentResultUnmarksKeyFocusForLowerRisk(t *testing.T) {
	item := domain.NewTestee(1, "testee", domain.GenderUnknown, nil)
	item.SetID(domain.ID(11))
	item.SetTags([]domain.Tag{domain.TagRiskHigh, domain.TagRiskSevere})
	item.SetKeyFocus(true)
	repo := &taggingRepoStub{item: item}
	service := NewTaggingService(
		repo,
		domain.NewRiskTagPolicy(),
		apptransaction.RunnerFunc(func(ctx context.Context, fn func(txCtx context.Context) error) error {
			return fn(ctx)
		}),
	)

	result, err := service.TagByAssessmentResult(context.Background(), 11, "low", "scale-a", nil, false)
	if err != nil {
		t.Fatalf("TagByAssessmentResult returned error: %v", err)
	}

	assertStrings(t, result.TagsRemoved, []string{"risk_high", "risk_severe"})
	assertStrings(t, result.TagsAdded, nil)
	if result.KeyFocusMarked || item.IsKeyFocus() {
		t.Fatalf("expected lower risk result to unmark key focus")
	}
	if repo.updateCalls != 1 {
		t.Fatalf("expected aggregate update, got %d", repo.updateCalls)
	}
}

type taggingRepoStub struct {
	item          *domain.Testee
	findByIDCalls int
	updateCalls   int
}

func (s *taggingRepoStub) Save(context.Context, *domain.Testee) error { return nil }

func (s *taggingRepoStub) Update(_ context.Context, item *domain.Testee) error {
	s.updateCalls++
	s.item = item
	return nil
}

func (s *taggingRepoStub) FindByID(_ context.Context, id domain.ID) (*domain.Testee, error) {
	s.findByIDCalls++
	return s.item, nil
}

func (s *taggingRepoStub) FindByProfile(context.Context, int64, uint64) (*domain.Testee, error) {
	return nil, nil
}

func (s *taggingRepoStub) Delete(context.Context, domain.ID) error { return nil }

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
