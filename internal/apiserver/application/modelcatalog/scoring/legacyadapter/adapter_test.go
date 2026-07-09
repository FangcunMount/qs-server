package legacyadapter

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAssessmentModelFromMedicalScalePreservesScaleDefinitionPayload(t *testing.T) {
	t.Parallel()

	scale := newLegacyScale(t)
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

	model, err := AssessmentModelFromMedicalScale(scale, now)
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale: %v", err)
	}
	if model.Kind != domain.KindScale ||
		model.Algorithm != domain.AlgorithmScaleDefault ||
		model.ProductChannel != domain.ProductChannelMedicalScale ||
		model.Status != domain.ModelStatusPublished {
		t.Fatalf("model identity = %#v", model)
	}
	if model.Category != "adhd" || !reflect.DeepEqual(model.Tags, []string{"screening", "clinical"}) {
		t.Fatalf("model metadata category=%q tags=%v", model.Category, model.Tags)
	}
	got, err := ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		t.Fatalf("ScaleSnapshotFromDefinitionPayload: %v", err)
	}
	want := ScaleSnapshotFromMedicalScale(scale)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scale snapshot mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAssessmentModelFromCreateDTOUsesLegacyDTOContract(t *testing.T) {
	t.Parallel()

	model, err := AssessmentModelFromCreateDTO(shared.CreateScaleDTO{
		Code:                 "SCL_DTO",
		Title:                "DTO Scale",
		Description:          "created from old route dto",
		Category:             "adhd",
		Stages:               []string{"deep_assessment"},
		ApplicableAges:       []string{"school_child"},
		Reporters:            []string{"parent"},
		Tags:                 []string{"screening"},
		QuestionnaireCode:    "Q_DTO",
		QuestionnaireVersion: "1.0.0",
	}, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromCreateDTO: %v", err)
	}
	if model.Code != "SCL_DTO" || model.Title != "DTO Scale" || model.Binding.QuestionnaireCode != "Q_DTO" {
		t.Fatalf("model = %#v", model)
	}
	snapshot, err := ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		t.Fatalf("ScaleSnapshotFromDefinitionPayload: %v", err)
	}
	if snapshot.Code != "SCL_DTO" || snapshot.ScaleVersion != scaledefinition.DefaultScaleVersion ||
		snapshot.QuestionnaireVersion != "1.0.0" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestScaleResultFromAssessmentModelProjectsLegacyResponseShape(t *testing.T) {
	t.Parallel()

	scale := newLegacyScale(t)
	model, err := AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale: %v", err)
	}

	result, err := ScaleResultFromAssessmentModel(model)
	if err != nil {
		t.Fatalf("ScaleResultFromAssessmentModel: %v", err)
	}
	if result.Code != "SCL_LEGACY" || result.ScaleVersion != "1.0.0" ||
		result.Category != "adhd" || result.Status != "published" ||
		!reflect.DeepEqual(result.Tags, []string{"screening", "clinical"}) {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Factors) != 2 {
		t.Fatalf("factor count = %d", len(result.Factors))
	}
	cntFactor := result.Factors[1]
	if cntFactor.ScoringStrategy != "cnt" || cntFactor.RiskLevel != "low" {
		t.Fatalf("factor result = %#v", cntFactor)
	}
	if got := cntFactor.ScoringParams["cnt_option_contents"]; !reflect.DeepEqual(got, []string{"yes", "often"}) {
		t.Fatalf("cnt params = %#v", got)
	}
}

func TestAssessmentSnapshotPublisherCreatesModelAndReplacesPublishedSnapshot(t *testing.T) {
	t.Parallel()

	scale := newLegacyScale(t)
	modelRepo := &memoryAssessmentModelRepo{}
	publishedRepo := &memoryPublishedModelRepo{}
	publisher := NewAssessmentSnapshotPublisher(modelRepo, publishedRepo)
	publisher.Now = func() time.Time { return time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC) }

	if err := publisher.PublishAssessmentSnapshot(context.Background(), scale); err != nil {
		t.Fatalf("PublishAssessmentSnapshot: %v", err)
	}
	if !reflect.DeepEqual(modelRepo.calls, []string{"find:SCL_LEGACY", "create:SCL_LEGACY"}) {
		t.Fatalf("model repo calls = %v", modelRepo.calls)
	}
	if !reflect.DeepEqual(publishedRepo.calls, []string{"delete:scale:SCL_LEGACY", "save:SCL_LEGACY"}) {
		t.Fatalf("published repo calls = %v", publishedRepo.calls)
	}
	model := modelRepo.models["SCL_LEGACY"]
	want, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshot: %v", err)
	}
	if !reflect.DeepEqual(publishedRepo.snapshots["SCL_LEGACY"], want) {
		t.Fatalf("published snapshot mismatch\n got: %#v\nwant: %#v", publishedRepo.snapshots["SCL_LEGACY"], want)
	}
}

func TestAssessmentSnapshotPublisherUpdatesExistingModel(t *testing.T) {
	t.Parallel()

	scale := newLegacyScale(t)
	existing, err := AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale(existing): %v", err)
	}
	existing.ID = "existing-id"
	existing.Version = 9
	modelRepo := &memoryAssessmentModelRepo{models: map[string]*domain.AssessmentModel{existing.Code: existing}}
	publishedRepo := &memoryPublishedModelRepo{}
	publisher := NewAssessmentSnapshotPublisher(modelRepo, publishedRepo)
	publisher.Now = func() time.Time { return time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC) }

	if err := publisher.PublishAssessmentSnapshot(context.Background(), scale); err != nil {
		t.Fatalf("PublishAssessmentSnapshot: %v", err)
	}
	if !reflect.DeepEqual(modelRepo.calls, []string{"find:SCL_LEGACY", "update:SCL_LEGACY"}) {
		t.Fatalf("model repo calls = %v", modelRepo.calls)
	}
	updated := modelRepo.models["SCL_LEGACY"]
	if updated.ID != "existing-id" || updated.Version != 10 {
		t.Fatalf("updated model id/version = %q/%d, want existing-id/10", updated.ID, updated.Version)
	}
}

func TestAssessmentSnapshotPublisherCanPublishWithoutModelRepo(t *testing.T) {
	t.Parallel()

	publishedRepo := &memoryPublishedModelRepo{}
	publisher := NewAssessmentSnapshotPublisher(nil, publishedRepo)
	publisher.Now = func() time.Time { return time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC) }
	if err := publisher.PublishAssessmentSnapshot(context.Background(), newLegacyScale(t)); err != nil {
		t.Fatalf("PublishAssessmentSnapshot: %v", err)
	}
	if !reflect.DeepEqual(publishedRepo.calls, []string{"delete:scale:SCL_LEGACY", "save:SCL_LEGACY"}) {
		t.Fatalf("published repo calls = %v", publishedRepo.calls)
	}
}

func newLegacyScale(t *testing.T) *scaledefinition.MedicalScale {
	t.Helper()

	maxScore := 10.0
	total, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("TOTAL"),
		"Total",
		scaledefinition.WithIsTotalScore(true),
		scaledefinition.WithScoringStrategy(scaledefinition.ScoringStrategySum),
		scaledefinition.WithMaxScore(&maxScore),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(0, 10),
				scaledefinition.RiskLevelNone,
				"none",
				"keep",
			),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor(total): %v", err)
	}
	cnt, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("CNT"),
		"Count Factor",
		scaledefinition.WithQuestionCodes([]meta.Code{meta.NewCode("Q1"), meta.NewCode("Q2")}),
		scaledefinition.WithScoringStrategy(scaledefinition.ScoringStrategyCnt),
		scaledefinition.WithScoringParams(
			scaledefinition.NewScoringParams().WithCntOptionContents([]string{"yes", "often"}),
		),
		scaledefinition.WithMaxScore(&maxScore),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(0, 5),
				scaledefinition.RiskLevelLow,
				"low",
				"watch",
			),
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(5, 10),
				scaledefinition.RiskLevelHigh,
				"high",
				"act",
			),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor(cnt): %v", err)
	}
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCL_LEGACY"),
		"Legacy Scale",
		scaledefinition.WithDescription("legacy definition"),
		scaledefinition.WithScaleVersion("1.0.0"),
		scaledefinition.WithQuestionnaire(meta.NewCode("Q_LEGACY"), "1.0.0"),
		scaledefinition.WithStatus(scaledefinition.StatusPublished),
		scaledefinition.WithCategory(scaledefinition.CategoryADHD),
		scaledefinition.WithTags([]scaledefinition.Tag{scaledefinition.NewTag("screening"), scaledefinition.NewTag("clinical")}),
		scaledefinition.WithFactors([]*scaledefinition.Factor{total, cnt}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale: %v", err)
	}
	return scale
}

type memoryAssessmentModelRepo struct {
	models map[string]*domain.AssessmentModel
	calls  []string
}

func (r *memoryAssessmentModelRepo) Create(_ context.Context, model *domain.AssessmentModel) error {
	if r.models == nil {
		r.models = map[string]*domain.AssessmentModel{}
	}
	r.calls = append(r.calls, "create:"+model.Code)
	r.models[model.Code] = model
	return nil
}

func (r *memoryAssessmentModelRepo) Update(_ context.Context, model *domain.AssessmentModel) error {
	if r.models == nil {
		r.models = map[string]*domain.AssessmentModel{}
	}
	r.calls = append(r.calls, "update:"+model.Code)
	r.models[model.Code] = model
	return nil
}

func (r *memoryAssessmentModelRepo) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	r.calls = append(r.calls, "find:"+code)
	if r.models == nil {
		return nil, domain.ErrNotFound
	}
	model, ok := r.models[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return model, nil
}

func (r *memoryAssessmentModelRepo) List(context.Context, port.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (r *memoryAssessmentModelRepo) Delete(context.Context, string) error { return nil }

type memoryPublishedModelRepo struct {
	snapshots map[string]*port.PublishedModel
	calls     []string
	err       error
}

func (r *memoryPublishedModelRepo) Save(_ context.Context, snapshot *port.PublishedModel) error {
	if r.err != nil {
		return r.err
	}
	if r.snapshots == nil {
		r.snapshots = map[string]*port.PublishedModel{}
	}
	r.calls = append(r.calls, "save:"+snapshot.Code)
	r.snapshots[snapshot.Code] = snapshot
	return nil
}

func (r *memoryPublishedModelRepo) FindPublishedByModelCode(context.Context, domain.Kind, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *memoryPublishedModelRepo) FindLatestPublishedByModelCode(context.Context, domain.Kind, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *memoryPublishedModelRepo) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*port.PublishedModel, error) {
	return nil, domain.ErrNotFound
}

func (r *memoryPublishedModelRepo) ListPublished(context.Context, port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (r *memoryPublishedModelRepo) DeletePublished(_ context.Context, kind domain.Kind, code string) error {
	if r.err != nil {
		return r.err
	}
	r.calls = append(r.calls, "delete:"+string(kind)+":"+code)
	if r.snapshots != nil {
		delete(r.snapshots, code)
	}
	return nil
}

var _ port.ModelRepository = (*memoryAssessmentModelRepo)(nil)
var _ port.PublishedModelRepository = (*memoryPublishedModelRepo)(nil)
