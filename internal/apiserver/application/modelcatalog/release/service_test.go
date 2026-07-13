package release

import (
	"context"
	"testing"
	"time"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	questionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPublishReleaseIsIdempotentForPublishedModel(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "MEDICAL-SCALE", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Medical scale", Now: now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel() error = %v", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: "MEDICAL-QUESTIONNAIRE", QuestionnaireVersion: "v1"}, now); err != nil {
		t.Fatalf("BindQuestionnaire() error = %v", err)
	}
	if err := model.MarkPublished(now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkPublished() error = %v", err)
	}
	repo := &publishedReleaseModelRepo{model: model}
	service := Service{
		Transactions:   directTransactionRunner{},
		Models:         repo,
		Published:      noopPublishedModelRepo{},
		Authorizer:     allowReleaseAuthorizer{},
		Questionnaires: noopQuestionnaireLifecycle{},
	}

	result, err := service.PublishRelease(context.Background(), modelcatalog.ActorContext{}, model.Code)
	if err != nil {
		t.Fatalf("PublishRelease() error = %v", err)
	}
	if result.ModelStatus != "published" || result.QuestionnaireCode != "MEDICAL-QUESTIONNAIRE" || result.QuestionnaireVersion != "v1" {
		t.Fatalf("result = %#v", result)
	}
	if repo.findCalls != 1 {
		t.Fatalf("FindByCode calls = %d, want 1; an idempotent publish must not emit post-commit effects", repo.findCalls)
	}
}

type directTransactionRunner struct{}

var _ apptransaction.Runner = directTransactionRunner{}

func (directTransactionRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type publishedReleaseModelRepo struct {
	modelcatalogport.ModelRepository
	model     *domain.AssessmentModel
	findCalls int
}

func (r *publishedReleaseModelRepo) FindByCode(context.Context, string) (*domain.AssessmentModel, error) {
	r.findCalls++
	return r.model, nil
}

type noopPublishedModelRepo struct {
	modelcatalogport.PublishedModelRepository
}

type noopQuestionnaireLifecycle struct {
	questionnaire.QuestionnaireLifecycleService
}

type allowReleaseAuthorizer struct{}

func (allowReleaseAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}
