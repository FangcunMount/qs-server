package norming_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
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

type memoryNormRepo struct {
	tables map[string]*norm.Norm
}

func (r *memoryNormRepo) UpsertNorm(_ context.Context, table *norm.Norm) error {
	if r.tables == nil {
		r.tables = map[string]*norm.Norm{}
	}
	r.tables[table.TableVersion] = table
	return nil
}

func (r *memoryNormRepo) FindNorm(_ context.Context, version string) (*norm.Norm, error) {
	table, ok := r.tables[version]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return table, nil
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
	if snapshot.Kind != domain.KindBehavioralRating || snapshot.Algorithm != domain.AlgorithmBrief2 {
		t.Fatalf("model identity = %s/%s", snapshot.Kind, snapshot.Algorithm)
	}
	decoded, err := publishedRepo.FindLatestPublishedByModelCode(context.Background(), domain.KindBehavioralRating, created.Code)
	if err != nil {
		t.Fatalf("reload snapshot: %v", err)
	}
	runtimeSnapshot, err := behavioralsnapshot.ParsePublishedPayload(
		decoded.PayloadFormat,
		decoded.Code,
		decoded.Version,
		decoded.Title,
		decoded.Status,
		decoded.Payload,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if runtimeSnapshot.Norming == nil || runtimeSnapshot.Norming.Variant != "parent" {
		t.Fatalf("norming profile = %#v", runtimeSnapshot.Norming)
	}
}

func TestUpdateDefinitionStoresTargetDefinitionV2(t *testing.T) {
	t.Parallel()

	modelRepo := &memoryModelRepo{}
	svc := norming.NewService(norming.Dependencies{ModelRepo: modelRepo, NormRepo: &memoryNormRepo{}})

	created, err := svc.Create(context.Background(), norming.CreateInput{
		Code:  "BR-V2",
		Title: "BRIEF-2",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	definition := []byte(`{
		"dimensions": [
			{"code": "inhibit", "title": "Inhibit", "question_codes": ["q1"], "scoring_strategy": "sum"},
			{"code": "self_monitor", "title": "Self Monitor", "question_codes": ["q2"], "scoring_strategy": "sum"},
			{"code": "bri", "title": "BRI"},
			{"code": "gec", "title": "GEC"}
		],
		"brief2": {
			"norm_table_version": "2024",
			"index_codes": ["bri", "gec"],
			"composite_indexes": [
				{"code": "bri", "strategy": "sum", "children": ["inhibit", "self_monitor"]},
				{"code": "gec", "strategy": "sum", "children": ["bri"]}
			],
			"norms": [{"factor_code": "gec"}]
		}
	}`)
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, norming.DefinitionInput{Payload: definition}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	saved := modelRepo.models[created.Code]
	if saved.DefinitionV2 == nil {
		t.Fatal("DefinitionV2 is nil")
	}
	roles := map[string]factor.FactorRole{}
	for _, item := range saved.DefinitionV2.Measure.Factors {
		roles[item.Code] = item.ResolvedRole()
	}
	if roles["bri"] != factor.FactorRoleIndex || roles["gec"] != factor.FactorRoleIndex {
		t.Fatalf("roles = %#v", roles)
	}
	if saved.DefinitionV2.Measure.FactorGraph.ParentCode("inhibit") != "bri" {
		t.Fatalf("inhibit parent = %q", saved.DefinitionV2.Measure.FactorGraph.ParentCode("inhibit"))
	}
	if len(saved.DefinitionV2.Calibration.NormRefs) != 1 || saved.DefinitionV2.Calibration.NormRefs[0].NormTableVersion != "2024" {
		t.Fatalf("norm refs = %#v", saved.DefinitionV2.Calibration.NormRefs)
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
