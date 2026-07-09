package factor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAddFactorUsesAssessmentModelRepositoryWhenConfigured(t *testing.T) {
	ctx := context.Background()
	scaleRepo := &ruleFreezeScaleRepoStub{}
	model := newDraftAssessmentModelForFactorTest(t)
	modelRepo := &factorAssessmentModelRepoStub{model: model}
	publisher := &scaleEventPublisherStub{}
	svc := NewService(scaleRepo, nil, publisher, WithAssessmentModelRepository(modelRepo))

	got, err := svc.AddFactor(ctx, shared.AddFactorDTO{
		ScaleCode:     model.Code,
		Code:          "F2",
		Title:         "Factor 2",
		QuestionCodes: []string{"Q2"},
	})
	if err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}
	if scaleRepo.updateCount != 0 {
		t.Fatalf("legacy scale repo Update calls = %d, want 0", scaleRepo.updateCount)
	}
	if modelRepo.updateCount < 1 {
		t.Fatalf("model repo Update calls = %d, want at least 1", modelRepo.updateCount)
	}
	if len(model.DefinitionV2.Measure.Factors) != 2 {
		t.Fatalf("measure factors = %#v, want 2 factors", model.DefinitionV2.Measure.Factors)
	}
	if got == nil || len(got.Factors) != 2 {
		t.Fatalf("result factors = %#v, want 2 factors", got)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published event count = %d, want 1", len(publisher.events))
	}
}

func TestAddFactorForksPublishedAssessmentModelDraft(t *testing.T) {
	ctx := context.Background()
	model := newDraftAssessmentModelForFactorTest(t)
	model.Status = domain.ModelStatusPublished
	publishedAt := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	model.PublishedAt = &publishedAt
	modelRepo := &factorAssessmentModelRepoStub{model: model}
	svc := NewService(&ruleFreezeScaleRepoStub{}, nil, &scaleEventPublisherStub{}, WithAssessmentModelRepository(modelRepo))

	if _, err := svc.AddFactor(ctx, shared.AddFactorDTO{
		ScaleCode:     model.Code,
		Code:          "F2",
		Title:         "Factor 2",
		QuestionCodes: []string{"Q2"},
	}); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
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

func newDraftAssessmentModelForFactorTest(t *testing.T) *domain.AssessmentModel {
	t.Helper()
	f, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("F1"),
		"Factor 1",
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(scaledefinition.NewScoreRange(0, 10), scaledefinition.RiskLevelLow, "low", "watch"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor() error = %v", err)
	}
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCL_FACTOR"),
		"Factor Scale",
		scaledefinition.WithQuestionnaire(meta.NewCode("Q1"), "1.0"),
		scaledefinition.WithFactors([]*scaledefinition.Factor{f}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale() error = %v", err)
	}
	model.Code = "SCL_FACTOR"
	return model
}

type factorAssessmentModelRepoStub struct {
	model       *domain.AssessmentModel
	updateCount int
}

func (r *factorAssessmentModelRepoStub) Create(context.Context, *domain.AssessmentModel) error { return nil }

func (r *factorAssessmentModelRepoStub) Update(_ context.Context, model *domain.AssessmentModel) error {
	r.updateCount++
	r.model = model
	return nil
}

func (r *factorAssessmentModelRepoStub) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model == nil || r.model.Code != code {
		return nil, domain.ErrNotFound
	}
	return r.model, nil
}

func (r *factorAssessmentModelRepoStub) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *factorAssessmentModelRepoStub) Delete(context.Context, string) error { return nil }

var _ modelcatalogport.ModelRepository = (*factorAssessmentModelRepoStub)(nil)
