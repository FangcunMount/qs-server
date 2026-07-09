package taskperformance_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/taskperformance"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type memoryModelRepo struct {
	models map[string]*domain.AssessmentModel
}

func (r *memoryModelRepo) Create(_ context.Context, model *domain.AssessmentModel) error {
	if r.models == nil {
		r.models = map[string]*domain.AssessmentModel{}
	}
	r.models[model.Code] = model
	return nil
}

func (r *memoryModelRepo) Update(_ context.Context, model *domain.AssessmentModel) error {
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

func (r *memoryModelRepo) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (r *memoryModelRepo) List(context.Context, port.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *memoryModelRepo) Delete(context.Context, string) error { return nil }

type memoryPublishedRepo struct {
	snapshots map[string]*port.PublishedModel
}

func (r *memoryPublishedRepo) Save(_ context.Context, snapshot *port.PublishedModel) error {
	if r.snapshots == nil {
		r.snapshots = map[string]*port.PublishedModel{}
	}
	r.snapshots[snapshot.Code] = snapshot
	return nil
}

func (r *memoryPublishedRepo) FindPublishedByModelCode(context.Context, domain.Kind, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *memoryPublishedRepo) FindLatestPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*port.PublishedModel, error) {
	snapshot, ok := r.snapshots[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r *memoryPublishedRepo) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *memoryPublishedRepo) ListPublished(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (r *memoryPublishedRepo) DeletePublished(_ context.Context, _ domain.Kind, code string) error {
	delete(r.snapshots, code)
	return nil
}

func TestPublishCognitiveModelRoundTrip(t *testing.T) {
	t.Parallel()

	modelRepo := &memoryModelRepo{}
	publishedRepo := &memoryPublishedRepo{}
	svc := taskperformance.NewService(taskperformance.Dependencies{
		ModelRepo:     modelRepo,
		PublishedRepo: publishedRepo,
	})

	created, err := svc.Create(context.Background(), taskperformance.CreateInput{
		Code:  "COG-001",
		Title: "认知测评",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != "draft" {
		t.Fatalf("status = %q, want draft", created.Status)
	}

	definition := []byte(`{
		"dimensions": [{
			"code": "total",
			"title": "总分",
			"question_codes": ["q1"],
			"scoring_strategy": "sum",
			"is_total_score": true
		}],
		"interpret_rules": [{
			"dimension_code": "total",
			"ranges": [{"min_score": 0, "max_score": 10, "conclusion": "ok"}]
		}]
	}`)
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, taskperformance.DefinitionInput{Payload: definition}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	if _, err := svc.BindQuestionnaire(context.Background(), taskperformance.BindQuestionnaireInput{
		Code:                 created.Code,
		QuestionnaireCode:    "SPM",
		QuestionnaireVersion: "1.0.0",
	}); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}

	published, err := svc.Publish(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published.Status != "published" {
		t.Fatalf("status = %q, want published", published.Status)
	}

	snapshot, err := publishedRepo.FindLatestPublishedByModelCode(context.Background(), domain.KindCognitive, created.Code)
	if err != nil {
		t.Fatalf("FindLatestPublishedByModelCode: %v", err)
	}
	if snapshot.PayloadFormat != domain.PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("payload format = %q, want %q", snapshot.PayloadFormat, domain.PayloadFormatCognitiveDefaultV1)
	}
	if snapshot.Kind != domain.KindCognitive || snapshot.Algorithm != domain.AlgorithmSPM {
		t.Fatalf("model identity = %s/%s", snapshot.Kind, snapshot.Algorithm)
	}
	if snapshot.DecisionKind != domain.DecisionKindAbilityLevel {
		t.Fatalf("decision kind = %s, want %s", snapshot.DecisionKind, domain.DecisionKindAbilityLevel)
	}
}

func TestUpdateDefinitionStoresTargetDefinitionV2(t *testing.T) {
	t.Parallel()

	modelRepo := &memoryModelRepo{}
	svc := taskperformance.NewService(taskperformance.Dependencies{ModelRepo: modelRepo})

	created, err := svc.Create(context.Background(), taskperformance.CreateInput{
		Code:  "COG-V2",
		Title: "认知测评",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload := []byte(`{
		"dimensions": [{
			"code": "total",
			"title": "总分",
			"question_codes": ["q1"],
			"scoring_strategy": "sum",
			"is_total_score": true
		}],
		"spm": {
			"item_set_codes": ["total"],
			"norm_table_version": "spm-cn-2026"
		}
	}`)
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, taskperformance.DefinitionInput{Payload: payload}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	saved := modelRepo.models[created.Code]
	if saved.DefinitionV2 == nil {
		t.Fatal("DefinitionV2 is nil")
	}
	if len(saved.DefinitionV2.Measure.Factors) != 1 || saved.DefinitionV2.Measure.Factors[0].Code != "total" {
		t.Fatalf("measure factors = %#v", saved.DefinitionV2.Measure.Factors)
	}
	if len(saved.DefinitionV2.Calibration.NormRefs) != 1 || saved.DefinitionV2.Calibration.NormRefs[0].NormTableVersion != "spm-cn-2026" {
		t.Fatalf("norm refs = %#v", saved.DefinitionV2.Calibration.NormRefs)
	}
}
