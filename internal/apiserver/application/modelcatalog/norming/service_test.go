package norming_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"
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

func (r *memoryModelRepo) List(context.Context, port.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *memoryModelRepo) Delete(context.Context, string) error { return nil }

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

func (r *memoryPublishedRepo) FindPublishedByModelCode(context.Context, domain.Kind, string) (*domain.PublishedModelSnapshot, error) {
	return nil, domain.ErrNotFound
}

func (r *memoryPublishedRepo) FindLatestPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	snapshot, ok := r.snapshots[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r *memoryPublishedRepo) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*domain.PublishedModelSnapshot, error) {
	return nil, domain.ErrNotFound
}

func (r *memoryPublishedRepo) ListPublished(context.Context, port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	return nil, 0, nil
}

func (r *memoryPublishedRepo) DeletePublished(_ context.Context, _ domain.Kind, code string) error {
	delete(r.snapshots, code)
	return nil
}

func TestPublishBehavioralRatingModelRoundTrip(t *testing.T) {
	t.Parallel()

	modelRepo := &memoryModelRepo{}
	publishedRepo := &memoryPublishedRepo{}
	svc := norming.NewService(norming.Dependencies{
		ModelRepo:     modelRepo,
		PublishedRepo: publishedRepo,
	})

	created, err := svc.Create(context.Background(), norming.CreateInput{
		Code:  "BR-001",
		Title: "BRIEF-2",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != "draft" {
		t.Fatalf("status = %q, want draft", created.Status)
	}
	if created.Algorithm != string(domain.AlgorithmBrief2) {
		t.Fatalf("algorithm = %q, want brief2", created.Algorithm)
	}

	definition := []byte(`{
		"dimensions": [{
			"code": "gec",
			"title": "GEC",
			"question_codes": ["q1"],
			"scoring_strategy": "sum",
			"is_total_score": true
		}],
		"interpret_rules": [{
			"dimension_code": "gec",
			"ranges": [{"min_score": 0, "max_score": 10, "conclusion": "ok"}]
		}],
		"brief2": {
			"form_variant": "parent",
			"primary_dimension_code": "gec",
			"norm_table_version": "2024",
			"index_codes": ["bri", "eri", "cri", "gec"],
			"validity_codes": ["inconsistency", "negativity"]
		}
	}`)
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, norming.DefinitionInput{Payload: definition}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	if _, err := svc.BindQuestionnaire(context.Background(), norming.BindQuestionnaireInput{
		Code:                 created.Code,
		QuestionnaireCode:    "MBRIEF2",
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

	snapshot, err := publishedRepo.FindLatestPublishedByModelCode(context.Background(), domain.KindBehavioralRating, created.Code)
	if err != nil {
		t.Fatalf("FindLatestPublishedByModelCode: %v", err)
	}
	if snapshot.PayloadFormat != domain.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("payload format = %q, want %q", snapshot.PayloadFormat, domain.PayloadFormatBehavioralRatingDefaultV1)
	}
	if snapshot.Model.Kind != domain.KindBehavioralRating || snapshot.Model.Algorithm != domain.AlgorithmBrief2 {
		t.Fatalf("model identity = %#v", snapshot.Model)
	}
	decoded, err := publishedRepo.FindLatestPublishedByModelCode(context.Background(), domain.KindBehavioralRating, created.Code)
	if err != nil {
		t.Fatalf("reload snapshot: %v", err)
	}
	runtimeSnapshot, err := behavioralsnapshot.ParsePublishedPayload(
		decoded.PayloadFormat,
		decoded.Model.Code,
		decoded.Model.Version,
		decoded.Model.Title,
		decoded.Model.Status,
		decoded.Payload,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if runtimeSnapshot.Norming == nil || runtimeSnapshot.Norming.Variant != "parent" {
		t.Fatalf("norming profile = %#v", runtimeSnapshot.Norming)
	}
}

func TestPublishRejectsInvalidFactorHierarchy(t *testing.T) {
	t.Parallel()

	modelRepo := &memoryModelRepo{}
	publishedRepo := &memoryPublishedRepo{}
	svc := norming.NewService(norming.Dependencies{
		ModelRepo:     modelRepo,
		PublishedRepo: publishedRepo,
	})

	created, err := svc.Create(context.Background(), norming.CreateInput{
		Code:  "BR-BAD-HIER",
		Title: "无效层级",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	definition := []byte(`{
		"dimensions": [{
			"code": "bri",
			"title": "BRI",
			"role": "index",
			"parent_code": "gec"
		}]
	}`)
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, norming.DefinitionInput{Payload: definition}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	if _, err := svc.BindQuestionnaire(context.Background(), norming.BindQuestionnaireInput{
		Code:                 created.Code,
		QuestionnaireCode:    "Q-001",
		QuestionnaireVersion: "1.0.0",
	}); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}

	if _, err := svc.Publish(context.Background(), created.Code); err == nil {
		t.Fatal("Publish() should reject invalid factor hierarchy")
	}
}
