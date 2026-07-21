package publication_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPublisherBuildSnapshotUsesDefinitionHandler(t *testing.T) {
	t.Parallel()

	model := newPublishedTestModel(t)
	publisher := publication.Publisher{
		Registry: definition.NewRegistry(snapshotHandler{}),
	}
	snapshot, err := publisher.BuildSnapshot(context.Background(), model)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if snapshot.Kind != domain.KindCognitive ||
		snapshot.Algorithm != domain.AlgorithmSPM ||
		snapshot.AlgorithmFamily != domain.AlgorithmFamilyTaskPerformance ||
		snapshot.DecisionKind != domain.DecisionKindAbilityLevel ||
		snapshot.Version != "v3" ||
		snapshot.QuestionnaireVersion != "1.0.0" || snapshot.DefinitionV2 == nil {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestPublisherPublishLeavesTransactionRollbackToCallerWhenDraftUpdateFails(t *testing.T) {
	t.Parallel()

	model := newPublishedTestModel(t)
	modelRepo := &publishedModelRepo{updateErr: errors.New("draft update failed")}
	publishedRepo := &publishedRepo{}
	publisher := publication.Publisher{
		Registry:  definition.NewRegistry(snapshotHandler{}),
		ModelRepo: modelRepo,
		Repo:      publishedRepo,
		Now:       func() time.Time { return time.Date(2026, 7, 9, 1, 2, 3, 0, time.UTC) },
	}
	if _, err := publisher.Publish(context.Background(), model, publication.PublishOptions{ReplaceKind: domain.KindCognitive}); err == nil {
		t.Fatal("Publish() error = nil, want draft update error")
	}
	if len(publishedRepo.snapshots) != 1 {
		t.Fatalf("snapshots = %#v, want transaction caller to roll back persisted snapshot", publishedRepo.snapshots)
	}
	if !reflect.DeepEqual(publishedRepo.calls, []string{"save:SPM"}) {
		t.Fatalf("published calls = %#v", publishedRepo.calls)
	}
}

func TestPublisherAllowsNonBlockingValidationWarnings(t *testing.T) {
	model := newPublishedTestModel(t)
	publisher := publication.Publisher{
		Registry:  definition.NewRegistry(warningSnapshotHandler{}),
		ModelRepo: &publishedModelRepo{}, Repo: &publishedRepo{},
	}
	if _, err := publisher.Publish(context.Background(), model, publication.PublishOptions{}); err != nil {
		t.Fatalf("Publish() with warning: %v", err)
	}
}

func TestPublisherPublishAttachesDefinitionHash(t *testing.T) {
	t.Parallel()
	model := newPublishedTestModel(t)
	model.DefinitionV2 = completeScaleDefinitionForPublishTest()
	repo := &publishedRepo{}
	publisher := publication.Publisher{
		Registry:  definition.NewRegistry(warningSnapshotHandler{}),
		ModelRepo: &publishedModelRepo{}, Repo: repo,
	}
	if _, err := publisher.Publish(context.Background(), model, publication.PublishOptions{}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	snapshot := repo.snapshots[model.Code]
	if snapshot == nil || snapshot.Source[port.SourceDefinitionContentHash] == "" || snapshot.Source[port.SourceDefinitionHashSchema] == "" {
		t.Fatalf("definition hash = %#v", snapshot.Source)
	}
}

func completeScaleDefinitionForPublishTest() *modeldefinition.Definition {
	return &modeldefinition.Definition{
		Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total"}},
	}
}

type snapshotHandler struct{}

type warningSnapshotHandler struct{ snapshotHandler }

func (warningSnapshotHandler) ValidateForPublish(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue {
	return []domain.DomainValidationIssue{{Code: "question_contribution.legacy_implicit", Message: "legacy", Level: domain.ValidationLevelWarning}}
}

func (snapshotHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindCognitive
}

func (snapshotHandler) ValidateForPublish(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue {
	return nil
}

func (snapshotHandler) MaterializeSnapshot(_ context.Context, _ *domain.AssessmentModel) (definition.Materialization, error) {
	return definition.Materialization{
		Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM,
		AlgorithmFamily: domain.AlgorithmFamilyTaskPerformance, DecisionKind: domain.DecisionKindAbilityLevel,
	}, nil
}

type publishedModelRepo struct {
	updateErr error
}

func (r *publishedModelRepo) Create(context.Context, *domain.AssessmentModel) error { return nil }

func (r *publishedModelRepo) Update(context.Context, *domain.AssessmentModel) error {
	return r.updateErr
}

func (r *publishedModelRepo) FindByCode(context.Context, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishedModelRepo) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishedModelRepo) List(context.Context, port.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *publishedModelRepo) Delete(context.Context, string) error { return nil }

type publishedRepo struct {
	snapshots map[string]*port.PublishedModel
	calls     []string
}

func (r *publishedRepo) Save(_ context.Context, snapshot *port.PublishedModel) error {
	if r.snapshots == nil {
		r.snapshots = map[string]*port.PublishedModel{}
	}
	r.calls = append(r.calls, "save:"+snapshot.Code)
	r.snapshots[snapshot.Code] = snapshot
	return nil
}

func (r *publishedRepo) FindPublishedByModelCode(context.Context, domain.Kind, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishedRepo) FindLatestPublishedByModelCode(context.Context, domain.Kind, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishedRepo) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *publishedRepo) ListPublished(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (r *publishedRepo) DeletePublished(_ context.Context, _ domain.Kind, code string) error {
	r.calls = append(r.calls, "delete:"+code)
	delete(r.snapshots, code)
	return nil
}

func newPublishedTestModel(t *testing.T) *domain.AssessmentModel {
	t.Helper()

	now := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:           "SPM",
		Kind:           domain.KindCognitive,
		Algorithm:      domain.AlgorithmSPM,
		ProductChannel: domain.ProductChannelBehaviorAbility,
		Title:          "SPM",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode:    "Q-SPM",
		QuestionnaireVersion: "1.0.0",
	}, now); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}
	if err := model.UpdateDefinition(&modeldefinition.Definition{Conclusions: []conclusion.Conclusion{conclusion.AbilityConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw}}}, now); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	return model
}
