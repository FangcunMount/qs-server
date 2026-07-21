package release

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/lifecycle"
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
		Published:      &idempotentPublishedRepo{model: model},
		Authorizer:     allowReleaseAuthorizer{},
		Questionnaires: noopQuestionnaireLifecycle{},
		QuestionnaireQuery: &idempotentQuestionnaireQuery{
			code: "MEDICAL-QUESTIONNAIRE", version: "v1", status: "published",
		},
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

func TestPublishReleaseRejectsIncompletePublishedPair(t *testing.T) {
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
	service := Service{
		Transactions:       directTransactionRunner{},
		Models:             &publishedReleaseModelRepo{model: model},
		Published:          noopPublishedModelRepo{},
		Authorizer:         allowReleaseAuthorizer{},
		Questionnaires:     noopQuestionnaireLifecycle{},
		QuestionnaireQuery: &idempotentQuestionnaireQuery{},
	}
	if _, err := service.PublishRelease(context.Background(), modelcatalog.ActorContext{}, model.Code); err == nil {
		t.Fatal("PublishRelease() error = nil, want incomplete pair conflict")
	}
}

func TestUnpublishReleaseArchivesPairAndKeepsHeadEditable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{Code: "MODEL-1", Kind: domain.KindTypology, Algorithm: domain.AlgorithmPersonalityTypology, Title: "Model", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0"}, now); err != nil {
		t.Fatal(err)
	}
	if err := model.MarkPublished(now); err != nil {
		t.Fatal(err)
	}
	models := &publishedReleaseModelRepo{model: model}
	published := &unpublishPublishedRepo{}
	questionnaires := &unpublishQuestionnaireLifecycle{}
	service := Service{Transactions: directTransactionRunner{}, Models: models, Published: published, Authorizer: allowReleaseAuthorizer{}, Questionnaires: questionnaires, Now: func() time.Time { return now.Add(time.Hour) }}

	result, err := service.UnpublishRelease(context.Background(), modelcatalog.ActorContext{}, model.Code)
	if err != nil {
		t.Fatalf("UnpublishRelease() error = %v", err)
	}
	if !model.IsDraft() || result.ModelStatus != "draft" {
		t.Fatalf("model status = %s, result = %#v", model.Status, result)
	}
	if !published.deleted || !questionnaires.called || !questionnaires.invalidated || models.updateCalls != 1 {
		t.Fatalf("transition calls = published:%t questionnaire:%t invalidated:%t updates:%d", published.deleted, questionnaires.called, questionnaires.invalidated, models.updateCalls)
	}
}

func TestArchiveReleaseArchivesActiveSnapshotWhenHeadHasDraftChanges(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{Code: "MODEL-1", Kind: domain.KindTypology, Algorithm: domain.AlgorithmPersonalityTypology, Title: "Model", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0"}, now); err != nil {
		t.Fatal(err)
	}
	// The head remains draft, representing edits made after the active release.
	models := &publishedReleaseModelRepo{model: model}
	published := &unpublishPublishedRepo{}
	questionnaires := &archiveQuestionnaireLifecycle{}
	service := Service{Transactions: directTransactionRunner{}, Models: models, Published: published, Authorizer: allowReleaseAuthorizer{}, Questionnaires: questionnaires, Now: func() time.Time { return now.Add(time.Hour) }}

	result, err := service.ArchiveRelease(context.Background(), modelcatalog.ActorContext{}, model.Code)
	if err != nil {
		t.Fatalf("ArchiveRelease() error = %v", err)
	}
	if !model.IsArchived() || result.ModelStatus != "archived" {
		t.Fatalf("model status = %s, result = %#v", model.Status, result)
	}
	if !published.deleted || !questionnaires.called {
		t.Fatalf("archive calls = published:%t questionnaire:%t", published.deleted, questionnaires.called)
	}
}

func TestReleaseEffectsDoNotRunWhenTransactionRollsBack(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 17, 11, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{Code: "MODEL-1", Kind: domain.KindTypology, Algorithm: domain.AlgorithmPersonalityTypology, Title: "Model", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: "Q-1", QuestionnaireVersion: "1"}, now); err != nil {
		t.Fatal(err)
	}
	if err := model.MarkPublished(now); err != nil {
		t.Fatal(err)
	}
	effectCalls := 0
	effects := lifecycle.NewEffectsRegistry(lifecycle.EffectFunc{
		Match: func(domain.Identity) bool { return true },
		Run:   func(context.Context, *domain.AssessmentModel, lifecycle.Action) { effectCalls++ },
	})
	questionnaires := &unpublishQuestionnaireLifecycle{}
	service := Service{
		Transactions: rollbackAfterCallbackRunner{}, Models: &publishedReleaseModelRepo{model: model},
		Published: &unpublishPublishedRepo{}, Authorizer: allowReleaseAuthorizer{},
		Questionnaires: questionnaires, Effects: effects,
	}
	if _, err := service.UnpublishRelease(context.Background(), modelcatalog.ActorContext{}, model.Code); err == nil {
		t.Fatal("UnpublishRelease() error = nil, want rollback")
	}
	if effectCalls != 0 {
		t.Fatalf("post-commit effect calls = %d, want 0", effectCalls)
	}
	if questionnaires.invalidated {
		t.Fatal("questionnaire cache invalidated before a failed commit")
	}
}

type directTransactionRunner struct{}

var _ apptransaction.Runner = directTransactionRunner{}

func (directTransactionRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type rollbackAfterCallbackRunner struct{}

func (rollbackAfterCallbackRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		return err
	}
	return stderrors.New("commit failed")
}

type publishedReleaseModelRepo struct {
	modelcatalogport.ModelRepository
	model       *domain.AssessmentModel
	findCalls   int
	updateCalls int
}

func (r *publishedReleaseModelRepo) Update(_ context.Context, model *domain.AssessmentModel) error {
	r.model = model
	r.updateCalls++
	return nil
}

func (r *publishedReleaseModelRepo) FindByCode(context.Context, string) (*domain.AssessmentModel, error) {
	r.findCalls++
	return r.model, nil
}

type noopPublishedModelRepo struct {
	modelcatalogport.PublishedSnapshotRepository
}

func (noopPublishedModelRepo) FindPublishedByModelCode(context.Context, domain.Kind, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

type idempotentPublishedRepo struct {
	modelcatalogport.PublishedSnapshotRepository
	model *domain.AssessmentModel
}

func (r *idempotentPublishedRepo) FindPublishedByModelCode(_ context.Context, kind domain.Kind, code string) (*modelcatalogport.PublishedModel, error) {
	if r.model == nil || r.model.Code != code || r.model.Kind != kind {
		return nil, domain.ErrNotFound
	}
	return &modelcatalogport.PublishedModel{
		Kind: r.model.Kind, Code: r.model.Code, Version: "1",
		QuestionnaireCode: r.model.Binding.QuestionnaireCode, QuestionnaireVersion: r.model.Binding.QuestionnaireVersion,
		ReleaseStatus: domain.ReleaseStatusActive, Status: "published",
	}, nil
}

type idempotentQuestionnaireQuery struct {
	questionnaire.QuestionnaireQueryService
	code, version, status string
}

func (q *idempotentQuestionnaireQuery) GetPublishedByCodeVersion(_ context.Context, code, version string) (*questionnaire.QuestionnaireResult, error) {
	if q.code == "" || code != q.code || version != q.version {
		return nil, stderrors.New("questionnaire not published")
	}
	return &questionnaire.QuestionnaireResult{Code: code, Version: version, Status: q.status}, nil
}

type noopQuestionnaireLifecycle struct {
	questionnaire.QuestionnaireLifecycleService
}

type unpublishPublishedRepo struct {
	modelcatalogport.PublishedSnapshotRepository
	deleted bool
}

func (r *unpublishPublishedRepo) DeletePublished(context.Context, domain.Kind, string) error {
	r.deleted = true
	return nil
}

type unpublishQuestionnaireLifecycle struct {
	questionnaire.QuestionnaireLifecycleService
	called      bool
	invalidated bool
}

func (s *unpublishQuestionnaireLifecycle) InvalidateReleaseCache(context.Context, string) {
	s.invalidated = true
}

type archiveQuestionnaireLifecycle struct {
	questionnaire.QuestionnaireLifecycleService
	called bool
}

func (s *archiveQuestionnaireLifecycle) ArchiveForRelease(_ context.Context, code string) (*questionnaire.QuestionnaireResult, error) {
	s.called = true
	return &questionnaire.QuestionnaireResult{Code: code, Version: "1.0.0", Status: "archived"}, nil
}

func (s *unpublishQuestionnaireLifecycle) UnpublishForRelease(_ context.Context, code string) (*questionnaire.QuestionnaireResult, error) {
	s.called = true
	return &questionnaire.QuestionnaireResult{Code: code, Version: "1.0.0", Status: "draft"}, nil
}

type allowReleaseAuthorizer struct{}

func (allowReleaseAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}
