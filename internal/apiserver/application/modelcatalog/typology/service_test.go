package typology_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	previewadapter "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/preview"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type memoryModelRepo struct {
	models     map[string]*domain.AssessmentModel
	updateErr  error
	updateHits int
}

func (r *memoryModelRepo) Create(_ context.Context, model *domain.AssessmentModel) error {
	if r.models == nil {
		r.models = map[string]*domain.AssessmentModel{}
	}
	if _, exists := r.models[model.Code]; exists {
		return domain.ErrInvalidArgument
	}
	r.models[model.Code] = cloneAssessmentModel(model)
	return nil
}

func (r *memoryModelRepo) Update(_ context.Context, model *domain.AssessmentModel) error {
	r.updateHits++
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.models == nil {
		return domain.ErrNotFound
	}
	if _, exists := r.models[model.Code]; !exists {
		return domain.ErrNotFound
	}
	r.models[model.Code] = cloneAssessmentModel(model)
	return nil
}

func (r *memoryModelRepo) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	model, ok := r.models[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneAssessmentModel(model), nil
}

func (r *memoryModelRepo) FindByQuestionnaireCode(_ context.Context, kind domain.Kind, questionnaireCode string) (*domain.AssessmentModel, error) {
	for _, model := range r.models {
		if model == nil || model.Binding.QuestionnaireCode != questionnaireCode {
			continue
		}
		if kind != "" && model.Kind != kind {
			continue
		}
		return cloneAssessmentModel(model), nil
	}
	return nil, domain.ErrNotFound
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
	snapshots  map[string]*port.PublishedModel
	deleteHits int
	deleteErr  error
}

func (r *memoryPublishedRepo) Save(_ context.Context, snapshot *port.PublishedModel) error {
	if r.snapshots == nil {
		r.snapshots = map[string]*port.PublishedModel{}
	}
	r.snapshots[snapshot.Code] = snapshot
	return nil
}

func (r *memoryPublishedRepo) FindPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*port.PublishedModel, error) {
	return r.FindLatestPublishedByModelCode(context.Background(), domain.KindTypology, code)
}

func (r *memoryPublishedRepo) FindLatestPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*port.PublishedModel, error) {
	snapshot, ok := r.snapshots[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r *memoryPublishedRepo) FindPublishedByModelCodeVersion(_ context.Context, _ domain.Kind, code, version string) (*port.PublishedModel, error) {
	snapshot, ok := r.snapshots[code]
	if !ok || snapshot.Version != version {
		return nil, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r *memoryPublishedRepo) ListPublished(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (r *memoryPublishedRepo) DeletePublished(_ context.Context, _ domain.Kind, code string) error {
	r.deleteHits++
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.snapshots, code)
	return nil
}

func cloneAssessmentModel(model *domain.AssessmentModel) *domain.AssessmentModel {
	if model == nil {
		return nil
	}
	cloned := *model
	cloned.Tags = append([]string(nil), model.Tags...)
	cloned.Definition.Data = append([]byte(nil), model.Definition.Data...)
	if model.PublishedAt != nil {
		publishedAt := *model.PublishedAt
		cloned.PublishedAt = &publishedAt
	}
	if model.ArchivedAt != nil {
		archivedAt := *model.ArchivedAt
		cloned.ArchivedAt = &archivedAt
	}
	return &cloned
}

type questionnaireQueryStub struct {
	questionnaire *questionnaireapp.QuestionnaireResult
	err           error
}

func (s questionnaireQueryStub) GetByCode(context.Context, string) (*questionnaireapp.QuestionnaireResult, error) {
	return s.questionnaire, s.err
}

func (s questionnaireQueryStub) List(context.Context, questionnaireapp.ListQuestionnairesDTO) (*questionnaireapp.QuestionnaireSummaryListResult, error) {
	return nil, nil
}

func (s questionnaireQueryStub) GetPublishedByCode(context.Context, string) (*questionnaireapp.QuestionnaireResult, error) {
	return s.questionnaire, s.err
}

func (s questionnaireQueryStub) GetPublishedByCodeVersion(context.Context, string, string) (*questionnaireapp.QuestionnaireResult, error) {
	return s.questionnaire, s.err
}

func (s questionnaireQueryStub) GetQuestionCount(context.Context, string) (int32, error) {
	if s.questionnaire == nil {
		return 0, s.err
	}
	return int32(len(s.questionnaire.Questions)), s.err
}

func (s questionnaireQueryStub) ListPublished(context.Context, questionnaireapp.ListQuestionnairesDTO) (*questionnaireapp.QuestionnaireSummaryListResult, error) {
	return nil, nil
}

func publishedQuestionnaire() *questionnaireapp.QuestionnaireResult {
	return &questionnaireapp.QuestionnaireResult{
		Code:    "Q_DEMO",
		Version: "1.0.0",
		Title:   "Demo Questionnaire",
		Status:  "published",
		Questions: []questionnaireapp.QuestionResult{{
			Code: "q1",
			Options: []questionnaireapp.OptionResult{
				{Value: "A", Label: "A"},
				{Value: "B", Label: "B"},
			},
		}},
	}
}

func frontendMBTIQuestionnaire() *questionnaireapp.QuestionnaireResult {
	return &questionnaireapp.QuestionnaireResult{
		Code:    "Q_FRONTEND_MBTI",
		Version: "1.0.0",
		Title:   "Frontend MBTI Questionnaire",
		Status:  "published",
		Questions: []questionnaireapp.QuestionResult{
			{Code: "Q_EI"},
			{Code: "Q_SN"},
			{Code: "Q_TF"},
			{Code: "Q_JP"},
		},
	}
}

func sampleRuntimePayload() []byte {
	return []byte(`{
		"algorithm":"mbti",
		"outcomes":[{"code":"INTJ","name":"建筑师","one_liner":"独立战略家"}],
		"runtime":{
			"factor_graph":{
				"factors":{
					"EI":{"id":"EI","code":"EI","name":"EI","kind":"leaf","contributions":[{"question_code":"q1","option_scores":{"A":1,"B":-1}}]}
				},
				"roots":["EI"]
			},
			"decision":{"kind":"pole_composition"},
			"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"personality_type"},
			"report":{"kind":"personality_type","adapter_key":"personality_type"}
		}
	}`)
}

func TestPreviewReportUsesDraftModelWithoutPublishing(t *testing.T) {
	payload, err := os.ReadFile("../../../testdata/personality/frontend_payload_mbti.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*port.PublishedModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: frontendMBTIQuestionnaire()},
		ReportPreviewer:    previewadapter.NewPreviewer(),
	})
	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_preview_mbti", Title: "Preview MBTI", Algorithm: "mbti",
		SubKind:              typology.SubKindTypology,
		QuestionnaireCode:    "Q_FRONTEND_MBTI",
		QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       payload,
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	previewPayload, err := json.Marshal(typology.PreviewReportInput{
		Answers: []typology.PreviewAnswer{
			{QuestionCode: "Q_EI", Score: floatPtr(1)},
			{QuestionCode: "Q_SN", Score: floatPtr(5)},
			{QuestionCode: "Q_TF", Score: floatPtr(1)},
			{QuestionCode: "Q_JP", Score: floatPtr(1)},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	result, err := svc.PreviewReport(context.Background(), created.Code, previewPayload)
	if err != nil {
		t.Fatalf("PreviewReport: %v", err)
	}
	if result.Outcome.Code != "INTJ" {
		t.Fatalf("outcome code = %s, want INTJ", result.Outcome.Code)
	}
	if len(result.ScoreDetail) == 0 {
		t.Fatal("score_detail is empty")
	}
	if len(result.ReportSections) == 0 {
		t.Fatal("report_sections is empty")
	}
	if result.RawReport == nil {
		t.Fatal("raw_report is nil")
	}
	if len(publishedRepo.snapshots) != 0 {
		t.Fatal("preview should not save published snapshot")
	}
	stored, err := modelRepo.FindByCode(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if stored.Status != domain.ModelStatusDraft {
		t.Fatalf("draft status = %s, want draft", stored.Status)
	}
}

func TestCreateAndPublishPersonalityModel(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*port.PublishedModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: publishedQuestionnaire()},
	})

	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_mbti_demo", Title: "Demo MBTI", Algorithm: "mbti",
		SubKind:           typology.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != typology.StatusDraft {
		t.Fatalf("status = %s", created.Status)
	}

	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       sampleRuntimePayload(),
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	published, err := svc.Publish(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published.Status != typology.StatusPublished {
		t.Fatalf("status = %s", published.Status)
	}
	if _, ok := publishedRepo.snapshots[created.Code]; !ok {
		t.Fatal("published snapshot was not saved")
	}
	stored, err := modelRepo.FindByCode(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	snapshot := publishedRepo.snapshots[created.Code]
	wantVersion := "v" + strconv.FormatInt(stored.Version, 10)
	if snapshot.Version != wantVersion {
		t.Fatalf("snapshot version = %s, want %s", snapshot.Version, wantVersion)
	}
}

func TestRepublishPersonalityModelAfterDefinitionChange(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*port.PublishedModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: publishedQuestionnaire()},
	})

	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_mbti_republish", Title: "Republish MBTI", Algorithm: "mbti",
		SubKind:           typology.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       sampleRuntimePayload(),
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	if _, err := svc.Publish(context.Background(), created.Code); err != nil {
		t.Fatalf("first Publish: %v", err)
	}
	firstDeleteHits := publishedRepo.deleteHits

	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       sampleRuntimePayload(),
	}); err != nil {
		t.Fatalf("UpdateDefinition after publish: %v", err)
	}
	if _, err := svc.Publish(context.Background(), created.Code); err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	if publishedRepo.deleteHits != firstDeleteHits+1 {
		t.Fatalf("deleteHits = %d, want %d", publishedRepo.deleteHits, firstDeleteHits+1)
	}
	if len(publishedRepo.snapshots) != 1 {
		t.Fatalf("published snapshots = %d, want 1", len(publishedRepo.snapshots))
	}
	stored, err := modelRepo.FindByCode(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	snapshot := publishedRepo.snapshots[created.Code]
	wantVersion := "v" + strconv.FormatInt(stored.Version, 10)
	if snapshot.Version != wantVersion {
		t.Fatalf("snapshot version = %s, want %s", snapshot.Version, wantVersion)
	}
}

func TestCreateWithQuestionnaireRequiresPublishedQuestionnaireWithQuestions(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      &memoryPublishedRepo{},
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: &questionnaireapp.QuestionnaireResult{Code: "Q_DEMO", Version: "1.0.0", Status: "published"}},
	})

	if _, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_create_empty_questionnaire", Title: "Empty Questionnaire", Algorithm: "mbti",
		SubKind:              typology.SubKindTypology,
		QuestionnaireCode:    "Q_DEMO",
		QuestionnaireVersion: "1.0.0",
	}); err == nil {
		t.Fatal("Create should fail when bound questionnaire has no questions")
	}
	if _, err := modelRepo.FindByCode(context.Background(), "personality_create_empty_questionnaire"); err == nil {
		t.Fatal("invalid model should not be persisted")
	}
}

func TestPublishPersonalityModelRequiresDefinition(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := typology.NewService(typology.Dependencies{ModelRepo: modelRepo, PublishedRepo: &memoryPublishedRepo{}})

	created, err := svc.Create(context.Background(), typology.CreateInput{
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
		Code: "personality_unpublish", Kind: domain.KindTypology,
		SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmMBTI, Title: "Unpublish", Now: now,
	})
	_ = model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: "Q", QuestionnaireVersion: "1"}, now)
	_ = model.UpdateDefinition(domain.DefinitionPayload{Format: domain.PayloadFormatPersonalityTypologyV1, Data: sampleRuntimePayload()}, now)
	_ = model.MarkPublished(now)

	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{model.Code: model}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*port.PublishedModel{
		model.Code: {Code: model.Code},
	}}
	svc := typology.NewService(typology.Dependencies{ModelRepo: modelRepo, PublishedRepo: publishedRepo})

	unpublished, err := svc.Unpublish(context.Background(), model.Code)
	if err != nil {
		t.Fatalf("Unpublish: %v", err)
	}
	if unpublished.Status != typology.StatusDraft {
		t.Fatalf("status = %s", unpublished.Status)
	}
	if _, ok := publishedRepo.snapshots[model.Code]; ok {
		t.Fatal("published snapshot should be removed")
	}
}

func TestUpdateDefinitionAllowsIncompleteDraft(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := typology.NewService(typology.Dependencies{ModelRepo: modelRepo, PublishedRepo: &memoryPublishedRepo{}})

	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_incomplete", Algorithm: "mbti", Title: "Incomplete",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       []byte(`{}`),
	}); err != nil {
		t.Fatalf("UpdateDefinition should allow incomplete draft payload: %v", err)
	}
}

func TestUpdateDefinitionStoresTargetDefinitionV2(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := typology.NewService(typology.Dependencies{ModelRepo: modelRepo, PublishedRepo: &memoryPublishedRepo{}})

	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_v2", Algorithm: "mbti", Title: "MBTI",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       sampleRuntimePayload(),
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	saved := modelRepo.models[created.Code]
	if saved.DefinitionV2 == nil {
		t.Fatal("DefinitionV2 is nil")
	}
	if len(saved.DefinitionV2.Measure.Factors) != 1 || saved.DefinitionV2.Measure.Factors[0].Code != "EI" {
		t.Fatalf("measure factors = %#v", saved.DefinitionV2.Measure.Factors)
	}
	if len(saved.DefinitionV2.Conclusions) != 1 {
		t.Fatalf("conclusions = %#v", saved.DefinitionV2.Conclusions)
	}
	if _, ok := saved.DefinitionV2.Conclusions[0].(domain.TypeConclusion); !ok {
		t.Fatalf("conclusion type = %T, want TypeConclusion", saved.DefinitionV2.Conclusions[0])
	}
	if len(saved.DefinitionV2.ReportMap.Sections) != 1 || saved.DefinitionV2.ReportMap.Sections[0].Code != "personality_type" {
		t.Fatalf("report map = %#v", saved.DefinitionV2.ReportMap)
	}
}

func TestBindQuestionnaireRequiresPublishedQuestionnaireWithQuestions(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      &memoryPublishedRepo{},
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: &questionnaireapp.QuestionnaireResult{Code: "Q_DEMO", Version: "1.0.0", Status: "published"}},
	})

	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_empty_questionnaire", Algorithm: "mbti", Title: "Empty Questionnaire",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.BindQuestionnaire(context.Background(), typology.BindQuestionnaireInput{
		Code: created.Code, QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	}); err == nil {
		t.Fatal("BindQuestionnaire should fail when questionnaire has no questions")
	}
}

func TestPublishRequiresQuestionReferencesInBoundQuestionnaire(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*port.PublishedModel{}}
	query := questionnaireQueryStub{questionnaire: publishedQuestionnaire()}
	svc := typology.NewService(typology.Dependencies{ModelRepo: modelRepo, PublishedRepo: publishedRepo, QuestionnaireQuery: query})

	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_bad_question_ref", Title: "Bad Ref", Algorithm: "mbti",
		SubKind:           typology.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload: []byte(`{
			"factor_graph":{"factors":{"EI":{"id":"EI","code":"EI","kind":"leaf","contributions":[{"question_code":"missing","option_scores":{"A":1}}]}},"roots":["EI"]},
			"decision":{"kind":"pole_composition"},
			"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"personality_type"},
			"report":{"kind":"personality_type","adapter_key":"personality_type"}
		}`),
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	if _, err := svc.Publish(context.Background(), created.Code); err == nil {
		t.Fatal("Publish should fail when definition references missing question")
	}
}

func TestPublishRequiresSupportedRuntimeAdapters(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*port.PublishedModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: publishedQuestionnaire()},
	})

	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_bad_adapter", Title: "Bad Adapter", Algorithm: "mbti",
		SubKind:           typology.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload: []byte(`{
			"algorithm":"mbti",
			"outcomes":[{"code":"INTJ","name":"建筑师"}],
			"runtime":{
				"factor_graph":{"factors":{"EI":{"id":"EI","code":"EI","kind":"leaf","contributions":[{"question_code":"q1","option_scores":{"A":1,"B":-1}}]}},"roots":["EI"]},
				"decision":{"kind":"pole_composition"},
				"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"mbti_default"},
				"report":{"kind":"personality_type","adapter_key":"mbti_default"}
			}
		}`),
	}); err != nil {
		t.Fatalf("UpdateDefinition should allow draft-only invalid adapter config: %v", err)
	}
	if _, err := svc.Publish(context.Background(), created.Code); err == nil {
		t.Fatal("Publish should fail when runtime adapters are unsupported")
	}
}

func TestPublishCompensatesWhenDraftUpdateFails(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*port.PublishedModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: publishedQuestionnaire()},
	})

	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_publish_compensate", Title: "Compensate", Algorithm: "mbti",
		SubKind:           typology.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       sampleRuntimePayload(),
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	modelRepo.updateErr = errors.New("draft update failed")
	if _, err := svc.Publish(context.Background(), created.Code); err == nil {
		t.Fatal("Publish should fail when draft update fails")
	}
	if _, ok := publishedRepo.snapshots[created.Code]; ok {
		t.Fatal("published snapshot should be compensated")
	}
	if publishedRepo.deleteHits == 0 {
		t.Fatal("DeletePublished should be called for compensation")
	}
}

func TestUnpublishDoesNotChangeDraftWhenPublishedDeleteFails(t *testing.T) {
	now := time.Now().UTC()
	model, _ := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "personality_unpublish_delete_failed", Kind: domain.KindTypology,
		SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmMBTI, Title: "Unpublish Delete Failed", Now: now,
	})
	_ = model.MarkPublished(now)
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{model.Code: cloneAssessmentModel(model)}}
	publishedRepo := &memoryPublishedRepo{
		snapshots: map[string]*port.PublishedModel{model.Code: {Code: model.Code}},
		deleteErr: errors.New("delete failed"),
	}
	svc := typology.NewService(typology.Dependencies{ModelRepo: modelRepo, PublishedRepo: publishedRepo})

	if _, err := svc.Unpublish(context.Background(), model.Code); err == nil {
		t.Fatal("Unpublish should fail when deleting published snapshot fails")
	}
	stored, err := modelRepo.FindByCode(context.Background(), model.Code)
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if stored.Status != domain.ModelStatusPublished {
		t.Fatalf("draft status = %s, want published", stored.Status)
	}
}

func TestArchiveDoesNotChangeDraftWhenPublishedDeleteFails(t *testing.T) {
	now := time.Now().UTC()
	model, _ := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "personality_archive_delete_failed", Kind: domain.KindTypology,
		SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmMBTI, Title: "Archive Delete Failed", Now: now,
	})
	_ = model.MarkPublished(now)
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{model.Code: cloneAssessmentModel(model)}}
	publishedRepo := &memoryPublishedRepo{
		snapshots: map[string]*port.PublishedModel{model.Code: {Code: model.Code}},
		deleteErr: errors.New("delete failed"),
	}
	svc := typology.NewService(typology.Dependencies{ModelRepo: modelRepo, PublishedRepo: publishedRepo})

	if _, err := svc.Archive(context.Background(), model.Code); err == nil {
		t.Fatal("Archive should fail when deleting published snapshot fails")
	}
	stored, err := modelRepo.FindByCode(context.Background(), model.Code)
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	if stored.Status != domain.ModelStatusPublished {
		t.Fatalf("draft status = %s, want published", stored.Status)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

func TestPreviewReportReturnsValidationIssuesWhenModelInvalid(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      &memoryPublishedRepo{},
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: frontendMBTIQuestionnaire()},
	})
	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_preview_invalid", Title: "Invalid Preview", Algorithm: "mbti",
		SubKind:              typology.SubKindTypology,
		QuestionnaireCode:    "Q_FRONTEND_MBTI",
		QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload, err := json.Marshal(typology.PreviewReportInput{
		Answers: []typology.PreviewAnswer{{QuestionCode: "Q_EI", Score: floatPtr(1)}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	_, err = svc.PreviewReport(context.Background(), created.Code, payload)
	if err == nil {
		t.Fatal("PreviewReport() error = nil, want validation failed")
	}
	issues, ok := typology.AsValidationFailed(err)
	if !ok {
		t.Fatalf("PreviewReport() error = %v, want validation failed", err)
	}
	if len(issues) == 0 {
		t.Fatal("validation issues is empty")
	}
}

func TestPreviewReportValidatesAnswers(t *testing.T) {
	payload, err := os.ReadFile("../../../testdata/personality/frontend_payload_mbti.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := typology.NewService(typology.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      &memoryPublishedRepo{},
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: frontendMBTIQuestionnaire()},
	})
	created, err := svc.Create(context.Background(), typology.CreateInput{
		Code: "personality_preview_answers", Title: "Preview Answers", Algorithm: "mbti",
		SubKind:              typology.SubKindTypology,
		QuestionnaireCode:    "Q_FRONTEND_MBTI",
		QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, typology.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       payload,
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	t.Run("unknown question code", func(t *testing.T) {
		body, err := json.Marshal(typology.PreviewReportInput{
			Answers: []typology.PreviewAnswer{{QuestionCode: "UNKNOWN", Score: floatPtr(1)}},
		})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		_, err = svc.PreviewReport(context.Background(), created.Code, body)
		issues, ok := typology.AsValidationFailed(err)
		if !ok {
			t.Fatalf("PreviewReport() error = %v, want validation failed", err)
		}
		if len(issues) == 0 || issues[0].Code != "question_code.not_found" {
			t.Fatalf("issues = %+v, want question_code.not_found", issues)
		}
	})

	t.Run("duplicate question code", func(t *testing.T) {
		body, err := json.Marshal(typology.PreviewReportInput{
			Answers: []typology.PreviewAnswer{
				{QuestionCode: "Q_EI", Score: floatPtr(1)},
				{QuestionCode: "Q_EI", Score: floatPtr(2)},
			},
		})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		_, err = svc.PreviewReport(context.Background(), created.Code, body)
		issues, ok := typology.AsValidationFailed(err)
		if !ok {
			t.Fatalf("PreviewReport() error = %v, want validation failed", err)
		}
		if len(issues) == 0 || issues[0].Code != "question_code.duplicate" {
			t.Fatalf("issues = %+v, want question_code.duplicate", issues)
		}
	})
}
