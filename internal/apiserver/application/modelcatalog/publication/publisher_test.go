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
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestDefaultSnapshotBuilderKeepsCurrentPublishContract(t *testing.T) {
	t.Parallel()

	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:      "PHQ9",
		Kind:      domain.KindScale,
		Algorithm: domain.AlgorithmScaleDefault,
		Title:     "PHQ-9",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.UpdateDefinition(domain.DefinitionPayload{Data: []byte(`{"code":"PHQ9"}`)}, model.CreatedAt); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	snapshot, err := publication.DefaultSnapshotBuilder(model)
	if err != nil {
		t.Fatalf("DefaultSnapshotBuilder: %v", err)
	}
	if snapshot.Kind != domain.KindScale || snapshot.PayloadFormat != domain.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

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
		snapshot.PayloadFormat != domain.PayloadFormatCognitiveDefaultV1 ||
		snapshot.DecisionKind != domain.DecisionKindAbilityLevel ||
		snapshot.Version != "v3" ||
		snapshot.QuestionnaireVersion != "1.0.0" ||
		string(snapshot.Payload) != `{"dimensions":[{"code":"total"}]}` {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestPublisherPublishCompensatesSnapshotWhenDraftUpdateFails(t *testing.T) {
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
	if len(publishedRepo.snapshots) != 0 {
		t.Fatalf("snapshots = %#v, want compensated empty store", publishedRepo.snapshots)
	}
	if !reflect.DeepEqual(publishedRepo.calls, []string{"delete:SPM", "save:SPM", "delete:SPM"}) {
		t.Fatalf("published calls = %#v", publishedRepo.calls)
	}
}

type snapshotHandler struct{}

func (snapshotHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindCognitive
}

func (snapshotHandler) PrepareForSave(context.Context, *domain.AssessmentModel, definition.SaveInput) (definition.SaveResult, []domain.DomainValidationIssue, error) {
	return definition.SaveResult{}, nil, nil
}

func (snapshotHandler) ValidateForPublish(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue {
	return nil
}

func (snapshotHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (definition.SnapshotBuildResult, error) {
	return definition.SnapshotBuildResult{
		Kind:          domain.KindCognitive,
		Algorithm:     domain.AlgorithmSPM,
		PayloadFormat: domain.PayloadFormatCognitiveDefaultV1,
		DecisionKind:  domain.DecisionKindAbilityLevel,
		Payload:       append([]byte(nil), model.Definition.Data...),
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
	if err := model.UpdateDefinition(domain.DefinitionPayload{
		Data: []byte(`{"dimensions":[{"code":"total"}]}`),
	}, now); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	return model
}
