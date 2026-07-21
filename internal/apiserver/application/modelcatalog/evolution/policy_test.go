package evolution_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/evolution"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPolicyAllowsChangesBeforeFirstPublish(t *testing.T) {
	t.Parallel()
	policy := evolution.Policy{History: historyStub{}}
	if err := policy.GuardAlgorithmChange(context.Background(), "M1", domain.AlgorithmPersonalityTypology); err != nil {
		t.Fatalf("GuardAlgorithmChange: %v", err)
	}
	if err := policy.GuardQuestionnaireCodeChange(context.Background(), "M1", "Q-NEW"); err != nil {
		t.Fatalf("GuardQuestionnaireCodeChange: %v", err)
	}
}

func TestPolicyFreezesAlgorithmAndQuestionnaireCode(t *testing.T) {
	t.Parallel()
	policy := evolution.Policy{History: historyStub{items: []*modelcatalogport.PublishedModel{
		{Code: "M1", Algorithm: domain.AlgorithmPersonalityTypology, QuestionnaireCode: "Q-A", Version: "2"},
		{Code: "M1", Algorithm: domain.AlgorithmPersonalityTypology, QuestionnaireCode: "Q-A", Version: "1"},
	}}}
	if err := policy.GuardAlgorithmChange(context.Background(), "M1", domain.AlgorithmPersonalityTypology); err != nil {
		t.Fatalf("same algorithm should pass: %v", err)
	}
	if err := policy.GuardAlgorithmChange(context.Background(), "M1", domain.AlgorithmBrief2); err == nil {
		t.Fatal("expected algorithm freeze rejection")
	}
	if err := policy.GuardQuestionnaireCodeChange(context.Background(), "M1", "Q-A"); err != nil {
		t.Fatalf("same questionnaire code should pass: %v", err)
	}
	if err := policy.GuardQuestionnaireCodeChange(context.Background(), "M1", "Q-B"); err == nil {
		t.Fatal("expected questionnaire code freeze rejection")
	}
}

func TestPolicyGuardPublishIdentity(t *testing.T) {
	t.Parallel()
	policy := evolution.Policy{History: historyStub{items: []*modelcatalogport.PublishedModel{
		{Code: "M1", Algorithm: domain.AlgorithmPersonalityTypology, QuestionnaireCode: "Q-A"},
	}}}
	model := &domain.AssessmentModel{
		Code: "M1", Algorithm: domain.AlgorithmPersonalityTypology,
		Binding: domain.QuestionnaireBinding{QuestionnaireCode: "Q-A", QuestionnaireVersion: "2"},
	}
	if err := policy.GuardPublishIdentity(context.Background(), model); err != nil {
		t.Fatalf("GuardPublishIdentity: %v", err)
	}
	model.Algorithm = domain.AlgorithmBrief2
	if err := policy.GuardPublishIdentity(context.Background(), model); err == nil {
		t.Fatal("expected publish identity rejection")
	}
}

type historyStub struct {
	items []*modelcatalogport.PublishedModel
	err   error
}

func (s historyStub) ListPublishedReleaseHistory(context.Context, string) ([]*modelcatalogport.PublishedModel, error) {
	return s.items, s.err
}
