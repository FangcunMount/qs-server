package personality_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/personality"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
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
	snapshots  map[string]*domain.PublishedModelSnapshot
	deleteHits int
	deleteErr  error
}

func (r *memoryPublishedRepo) Save(_ context.Context, snapshot *domain.PublishedModelSnapshot) error {
	if r.snapshots == nil {
		r.snapshots = map[string]*domain.PublishedModelSnapshot{}
	}
	r.snapshots[snapshot.Model.Code] = snapshot
	return nil
}

func (r *memoryPublishedRepo) FindPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	return r.FindLatestPublishedByModelCode(context.Background(), domain.KindPersonality, code)
}

func (r *memoryPublishedRepo) FindLatestPublishedByModelCode(_ context.Context, _ domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	snapshot, ok := r.snapshots[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r *memoryPublishedRepo) FindPublishedByModelCodeVersion(_ context.Context, _ domain.Kind, code, version string) (*domain.PublishedModelSnapshot, error) {
	snapshot, ok := r.snapshots[code]
	if !ok || snapshot.Model.Version != version {
		return nil, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r *memoryPublishedRepo) ListPublished(context.Context, port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
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
			"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"mbti"},
			"report":{"kind":"template","adapter_key":"mbti"}
		}
	}`)
}

func TestPreviewReportUsesDraftModelWithoutPublishing(t *testing.T) {
	payload, err := os.ReadFile("../../../testdata/personality/frontend_payload_mbti.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*domain.PublishedModelSnapshot{}}
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: frontendMBTIQuestionnaire()},
	})
	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_preview_mbti", Title: "Preview MBTI", Algorithm: "mbti",
		SubKind:              personality.SubKindTypology,
		QuestionnaireCode:    "Q_FRONTEND_MBTI",
		QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, personality.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       payload,
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	previewPayload, err := json.Marshal(personality.PreviewReportInput{
		Answers: []personality.PreviewAnswer{
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
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*domain.PublishedModelSnapshot{}}
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: publishedQuestionnaire()},
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
	stored, err := modelRepo.FindByCode(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	snapshot := publishedRepo.snapshots[created.Code]
	wantVersion := "v" + strconv.FormatInt(stored.Version, 10)
	if snapshot.Model.Version != wantVersion {
		t.Fatalf("snapshot version = %s, want %s", snapshot.Model.Version, wantVersion)
	}
}

func TestCreateWithQuestionnaireRequiresPublishedQuestionnaireWithQuestions(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      &memoryPublishedRepo{},
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: &questionnaireapp.QuestionnaireResult{Code: "Q_DEMO", Version: "1.0.0", Status: "published"}},
	})

	if _, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_create_empty_questionnaire", Title: "Empty Questionnaire", Algorithm: "mbti",
		SubKind:              personality.SubKindTypology,
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

func TestUpdateDefinitionAllowsIncompleteDraft(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := personality.NewService(personality.Dependencies{ModelRepo: modelRepo, PublishedRepo: &memoryPublishedRepo{}})

	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_incomplete", Algorithm: "mbti", Title: "Incomplete",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, personality.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       []byte(`{}`),
	}); err != nil {
		t.Fatalf("UpdateDefinition should allow incomplete draft payload: %v", err)
	}
}

func TestBindQuestionnaireRequiresPublishedQuestionnaireWithQuestions(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      &memoryPublishedRepo{},
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: &questionnaireapp.QuestionnaireResult{Code: "Q_DEMO", Version: "1.0.0", Status: "published"}},
	})

	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_empty_questionnaire", Algorithm: "mbti", Title: "Empty Questionnaire",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.BindQuestionnaire(context.Background(), personality.BindQuestionnaireInput{
		Code: created.Code, QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	}); err == nil {
		t.Fatal("BindQuestionnaire should fail when questionnaire has no questions")
	}
}

func TestPublishRequiresQuestionReferencesInBoundQuestionnaire(t *testing.T) {
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*domain.PublishedModelSnapshot{}}
	query := questionnaireQueryStub{questionnaire: publishedQuestionnaire()}
	svc := personality.NewService(personality.Dependencies{ModelRepo: modelRepo, PublishedRepo: publishedRepo, QuestionnaireQuery: query})

	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_bad_question_ref", Title: "Bad Ref", Algorithm: "mbti",
		SubKind:           personality.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, personality.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload: []byte(`{
			"factor_graph":{"factors":{"EI":{"id":"EI","code":"EI","kind":"leaf","contributions":[{"question_code":"missing","option_scores":{"A":1}}]}},"roots":["EI"]},
			"decision":{"kind":"pole_composition"},
			"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"mbti"},
			"report":{"kind":"template","adapter_key":"mbti"}
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
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*domain.PublishedModelSnapshot{}}
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: publishedQuestionnaire()},
	})

	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_bad_adapter", Title: "Bad Adapter", Algorithm: "mbti",
		SubKind:           personality.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, personality.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload: []byte(`{
			"algorithm":"mbti",
			"outcomes":[{"code":"INTJ","name":"建筑师"}],
			"runtime":{
				"factor_graph":{"factors":{"EI":{"id":"EI","code":"EI","kind":"leaf","contributions":[{"question_code":"q1","option_scores":{"A":1,"B":-1}}]}},"roots":["EI"]},
				"decision":{"kind":"pole_composition"},
				"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"mbti_default"},
				"report":{"kind":"template","adapter_key":"mbti_default"}
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
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*domain.PublishedModelSnapshot{}}
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: publishedQuestionnaire()},
	})

	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_publish_compensate", Title: "Compensate", Algorithm: "mbti",
		SubKind:           personality.SubKindTypology,
		QuestionnaireCode: "Q_DEMO", QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, personality.DefinitionInput{
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
		Code: "personality_unpublish_delete_failed", Kind: domain.KindPersonality,
		SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmMBTI, Title: "Unpublish Delete Failed", Now: now,
	})
	_ = model.MarkPublished(now)
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{model.Code: cloneAssessmentModel(model)}}
	publishedRepo := &memoryPublishedRepo{
		snapshots: map[string]*domain.PublishedModelSnapshot{model.Code: {Model: domain.ModelDefinition{Code: model.Code}}},
		deleteErr: errors.New("delete failed"),
	}
	svc := personality.NewService(personality.Dependencies{ModelRepo: modelRepo, PublishedRepo: publishedRepo})

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
		Code: "personality_archive_delete_failed", Kind: domain.KindPersonality,
		SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmMBTI, Title: "Archive Delete Failed", Now: now,
	})
	_ = model.MarkPublished(now)
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{model.Code: cloneAssessmentModel(model)}}
	publishedRepo := &memoryPublishedRepo{
		snapshots: map[string]*domain.PublishedModelSnapshot{model.Code: {Model: domain.ModelDefinition{Code: model.Code}}},
		deleteErr: errors.New("delete failed"),
	}
	svc := personality.NewService(personality.Dependencies{ModelRepo: modelRepo, PublishedRepo: publishedRepo})

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
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      &memoryPublishedRepo{},
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: frontendMBTIQuestionnaire()},
	})
	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_preview_invalid", Title: "Invalid Preview", Algorithm: "mbti",
		SubKind:              personality.SubKindTypology,
		QuestionnaireCode:    "Q_FRONTEND_MBTI",
		QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	payload, err := json.Marshal(personality.PreviewReportInput{
		Answers: []personality.PreviewAnswer{{QuestionCode: "Q_EI", Score: floatPtr(1)}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	_, err = svc.PreviewReport(context.Background(), created.Code, payload)
	if err == nil {
		t.Fatal("PreviewReport() error = nil, want validation failed")
	}
	issues, ok := personality.AsValidationFailed(err)
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
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      &memoryPublishedRepo{},
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: frontendMBTIQuestionnaire()},
	})
	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code: "personality_preview_answers", Title: "Preview Answers", Algorithm: "mbti",
		SubKind:              personality.SubKindTypology,
		QuestionnaireCode:    "Q_FRONTEND_MBTI",
		QuestionnaireVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.UpdateDefinition(context.Background(), created.Code, personality.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       payload,
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	t.Run("unknown question code", func(t *testing.T) {
		body, err := json.Marshal(personality.PreviewReportInput{
			Answers: []personality.PreviewAnswer{{QuestionCode: "UNKNOWN", Score: floatPtr(1)}},
		})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		_, err = svc.PreviewReport(context.Background(), created.Code, body)
		issues, ok := personality.AsValidationFailed(err)
		if !ok {
			t.Fatalf("PreviewReport() error = %v, want validation failed", err)
		}
		if len(issues) == 0 || issues[0].Code != "question_code.not_found" {
			t.Fatalf("issues = %+v, want question_code.not_found", issues)
		}
	})

	t.Run("duplicate question code", func(t *testing.T) {
		body, err := json.Marshal(personality.PreviewReportInput{
			Answers: []personality.PreviewAnswer{
				{QuestionCode: "Q_EI", Score: floatPtr(1)},
				{QuestionCode: "Q_EI", Score: floatPtr(2)},
			},
		})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		_, err = svc.PreviewReport(context.Background(), created.Code, body)
		issues, ok := personality.AsValidationFailed(err)
		if !ok {
			t.Fatalf("PreviewReport() error = %v, want validation failed", err)
		}
		if len(issues) == 0 || issues[0].Code != "question_code.duplicate" {
			t.Fatalf("issues = %+v, want question_code.duplicate", issues)
		}
	})
}
