package personality_test

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/personality"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

type memoryModelRepo struct {
	models map[string]*domain.AssessmentModel
}

func (r *memoryModelRepo) Create(_ context.Context, model *domain.AssessmentModel) error {
	if r.models == nil {
		r.models = map[string]*domain.AssessmentModel{}
	}
	if _, exists := r.models[model.Code]; exists {
		return domain.ErrInvalidArgument
	}
	r.models[model.Code] = model
	return nil
}

func (r *memoryModelRepo) Update(_ context.Context, model *domain.AssessmentModel) error {
	if r.models == nil {
		return domain.ErrNotFound
	}
	if _, exists := r.models[model.Code]; !exists {
		return domain.ErrNotFound
	}
	r.models[model.Code] = model
	return nil
}

func (r *memoryModelRepo) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	model, ok := r.models[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return model, nil
}

func (r *memoryModelRepo) List(_ context.Context, _ port.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	items := make([]*domain.AssessmentModel, 0, len(r.models))
	for _, model := range r.models {
		items = append(items, model)
	}
	return items, int64(len(items)), nil
}

func (r *memoryModelRepo) Delete(_ context.Context, code string) error {
	delete(r.models, code)
	return nil
}

type memoryPublishedRepo struct {
	snapshots map[string]*domain.PublishedModelSnapshot
}

func (r *memoryPublishedRepo) Save(_ context.Context, snapshot *domain.PublishedModelSnapshot) error {
	if r.snapshots == nil {
		r.snapshots = map[string]*domain.PublishedModelSnapshot{}
	}
	r.snapshots[snapshot.Model.Code] = snapshot
	return nil
}

func (r *memoryPublishedRepo) FindPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	snapshot, ok := r.snapshots[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r *memoryPublishedRepo) ListPublished(context.Context, port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	return nil, 0, nil
}

func (r *memoryPublishedRepo) DeletePublished(_ context.Context, _ domain.Kind, code string) error {
	delete(r.snapshots, code)
	return nil
}

func sampleRuntimePayload() []byte {
	return []byte(`{
		"factor_graph":{"dimension_order":["EI"],"dimensions":{"EI":{"code":"EI","name":"EI"}},"roots":["EI"]},
		"decision":{"kind":"pole_composition"},
		"outcome_mapping":{"detail_kind":"mbti_type"},
		"report":{"kind":"template","adapter_key":"mbti_default"}
	}`)
}

func TestCreateAndPublishPersonalityModel(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*domain.PublishedModelSnapshot{}}
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:     modelRepo,
		PublishedRepo: publishedRepo,
	})

	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_mbti_demo", Title: "Demo MBTI", Algorithm: "mbti",
		SubKind:           personality.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != personality.StatusDraft {
		t.Fatalf("status = %s", created.Status)
	}

	if _, err := svc.UpdateDefinition(context.Background(), created.Code, personality.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       sampleRuntimePayload(),
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	published, err := svc.Publish(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published.Status != personality.StatusPublished {
		t.Fatalf("status = %s", published.Status)
	}
	if _, ok := publishedRepo.snapshots[created.Code]; !ok {
		t.Fatal("published snapshot was not saved")
	}
}

func TestPublishPersonalityModelRequiresDefinition(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := personality.NewService(personality.Dependencies{ModelRepo: modelRepo, PublishedRepo: &memoryPublishedRepo{}})

	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_empty", Algorithm: "mbti", Title: "Empty",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Publish(context.Background(), created.Code); err == nil {
		t.Fatal("Publish should fail without definition")
	}
}

func TestUnpublishPersonalityModel(t *testing.T) {
	now := time.Now().UTC()
	model, _ := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "personality_unpublish", Kind: domain.KindPersonality,
		SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmMBTI, Title: "Unpublish", Now: now,
	})
	_ = model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: "Q", QuestionnaireVersion: "1"}, now)
	_ = model.UpdateDefinition(domain.DefinitionPayload{Format: domain.PayloadFormatPersonalityTypologyV1, Data: sampleRuntimePayload()}, now)
	_ = model.MarkPublished(now)

	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{model.Code: model}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*domain.PublishedModelSnapshot{
		model.Code: {Model: domain.ModelDefinition{Code: model.Code}},
	}}
	svc := personality.NewService(personality.Dependencies{ModelRepo: modelRepo, PublishedRepo: publishedRepo})

	unpublished, err := svc.Unpublish(context.Background(), model.Code)
	if err != nil {
		t.Fatalf("Unpublish: %v", err)
	}
	if unpublished.Status != personality.StatusDraft {
		t.Fatalf("status = %s", unpublished.Status)
	}
	if _, ok := publishedRepo.snapshots[model.Code]; ok {
		t.Fatal("published snapshot should be removed")
	}
}
