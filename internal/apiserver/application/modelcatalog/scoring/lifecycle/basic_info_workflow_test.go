package lifecycle

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestUpdateBasicInfoUsesAssessmentModelRepositoryWhenConfigured(t *testing.T) {
	ctx := context.Background()
	modelRepo := &basicInfoAssessmentModelRepoStub{
		model: newDraftScaleAssessmentModel(t),
	}
	svc := newAuthoringLifecycleService(nil, modelRepo, nil)

	got, err := svc.UpdateBasicInfo(ctx, shared.UpdateScaleBasicInfoDTO{
		Code:           "SCL_BASIC",
		Title:          "Updated Title",
		Description:    "updated description",
		Category:       "mental",
		Stages:         []string{"adult"},
		ApplicableAges: []string{"18+"},
		Reporters:      []string{"self"},
		Tags:           []string{"updated"},
	})
	if err != nil {
		t.Fatalf("UpdateBasicInfo() error = %v", err)
	}
	if modelRepo.updateCount != 1 {
		t.Fatalf("model repo Update calls = %d, want 1", modelRepo.updateCount)
	}
	model := modelRepo.model
	if model.Title != "Updated Title" || model.Description != "updated description" {
		t.Fatalf("updated model metadata = title %q description %q", model.Title, model.Description)
	}
	if !reflect.DeepEqual(model.Stages, []string{"adult"}) ||
		!reflect.DeepEqual(model.ApplicableAges, []string{"18+"}) ||
		!reflect.DeepEqual(model.Reporters, []string{"self"}) {
		t.Fatalf("audience metadata = stages %#v ages %#v reporters %#v", model.Stages, model.ApplicableAges, model.Reporters)
	}
	if got == nil || got.Title != "Updated Title" || got.Description != "updated description" {
		t.Fatalf("result = %#v, want updated scale result", got)
	}
}

func TestUpdateQuestionnaireUsesAssessmentModelRepositoryWhenConfigured(t *testing.T) {
	ctx := context.Background()
	modelRepo := &basicInfoAssessmentModelRepoStub{
		model: newDraftScaleAssessmentModel(t),
	}
	svc := newAuthoringLifecycleService(&medicalScaleQuestionnaireCatalogStub{}, modelRepo, nil)

	got, err := svc.UpdateQuestionnaire(ctx, shared.UpdateScaleQuestionnaireDTO{
		Code:                 "SCL_BASIC",
		QuestionnaireCode:    "Q-NEW",
		QuestionnaireVersion: "2.0",
	})
	if err != nil {
		t.Fatalf("UpdateQuestionnaire() error = %v", err)
	}
	if modelRepo.updateCount != 1 {
		t.Fatalf("model repo Update calls = %d, want 1", modelRepo.updateCount)
	}
	model := modelRepo.model
	if model.Binding.QuestionnaireCode != "Q-NEW" || model.Binding.QuestionnaireVersion != "2.0" {
		t.Fatalf("binding = %#v, want Q-NEW@2.0", model.Binding)
	}
	var snapshot scalesnapshot.ScaleSnapshot
	if err := json.Unmarshal(model.Definition.Data, &snapshot); err != nil {
		t.Fatalf("unmarshal definition payload: %v", err)
	}
	if snapshot.QuestionnaireCode != "Q-NEW" || snapshot.QuestionnaireVersion != "2.0" {
		t.Fatalf("payload binding = code %q version %q", snapshot.QuestionnaireCode, snapshot.QuestionnaireVersion)
	}
	if got == nil || got.QuestionnaireCode != "Q-NEW" || got.QuestionnaireVersion != "2.0" {
		t.Fatalf("result = %#v, want updated questionnaire binding", got)
	}
}

func TestUpdateBasicInfoForksPublishedAssessmentModelDraft(t *testing.T) {
	ctx := context.Background()
	model := newDraftScaleAssessmentModel(t)
	model.Status = domain.ModelStatusPublished
	publishedAt := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	model.PublishedAt = &publishedAt
	modelRepo := &basicInfoAssessmentModelRepoStub{model: model}
	svc := newAuthoringLifecycleService(nil, modelRepo, nil)

	if _, err := svc.UpdateBasicInfo(ctx, shared.UpdateScaleBasicInfoDTO{
		Code:  "SCL_BASIC",
		Title: "Published Fork",
	}); err != nil {
		t.Fatalf("UpdateBasicInfo() error = %v", err)
	}
	if model.Status != domain.ModelStatusDraft {
		t.Fatalf("status = %s, want draft after fork", model.Status)
	}
	var snapshot scalesnapshot.ScaleSnapshot
	if err := json.Unmarshal(model.Definition.Data, &snapshot); err != nil {
		t.Fatalf("unmarshal definition payload: %v", err)
	}
	if snapshot.ScaleVersion != "1.0.1" || snapshot.Status != scaledefinition.StatusDraft.String() {
		t.Fatalf("forked payload = version %q status %q, want 1.0.1 draft", snapshot.ScaleVersion, snapshot.Status)
	}
}

func newDraftScaleAssessmentModel(t *testing.T) *domain.AssessmentModel {
	t.Helper()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCL_BASIC"),
		"Basic Scale",
		scaledefinition.WithQuestionnaire(meta.NewCode("Q1"), "1.0"),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, now)
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale() error = %v", err)
	}
	model.Code = "SCL_BASIC"
	return model
}

type basicInfoAssessmentModelRepoStub struct {
	model       *domain.AssessmentModel
	updateCount int
}

func (r *basicInfoAssessmentModelRepoStub) Create(context.Context, *domain.AssessmentModel) error {
	return nil
}

func (r *basicInfoAssessmentModelRepoStub) Update(_ context.Context, model *domain.AssessmentModel) error {
	r.updateCount++
	r.model = model
	return nil
}

func (r *basicInfoAssessmentModelRepoStub) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model == nil || r.model.Code != code {
		return nil, domain.ErrNotFound
	}
	return r.model, nil
}

func (r *basicInfoAssessmentModelRepoStub) FindByQuestionnaireCode(_ context.Context, kind domain.Kind, questionnaireCode string) (*domain.AssessmentModel, error) {
	if r.model != nil && r.model.Binding.QuestionnaireCode == questionnaireCode && (kind == "" || r.model.Kind == kind) {
		return r.model, nil
	}
	return nil, domain.ErrNotFound
}

func (r *basicInfoAssessmentModelRepoStub) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *basicInfoAssessmentModelRepoStub) Delete(context.Context, string) error { return nil }

var _ modelcatalogport.ModelRepository = (*basicInfoAssessmentModelRepoStub)(nil)

type medicalScaleQuestionnaireCatalogStub struct{}

func (medicalScaleQuestionnaireCatalogStub) FindQuestionnaire(_ context.Context, code string) (*questionnairecatalog.Item, error) {
	return &questionnairecatalog.Item{Code: code, Type: "MedicalScale", Status: "draft"}, nil
}

func (medicalScaleQuestionnaireCatalogStub) FindQuestionnaireVersion(_ context.Context, code, version string) (*questionnairecatalog.Item, error) {
	return &questionnairecatalog.Item{Code: code, Version: version, Type: "MedicalScale", Status: "draft"}, nil
}

func (medicalScaleQuestionnaireCatalogStub) FindPublishedQuestionnaire(context.Context, string) (*questionnairecatalog.Item, error) {
	return nil, nil
}
