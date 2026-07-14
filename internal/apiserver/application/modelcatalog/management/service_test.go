package management

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestUpdateBasicInfoForScaleAdvancesRevisionOnce(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "SNAP-IV", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Before", Now: now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel() error = %v", err)
	}
	initialRevision := model.Revision()
	repo := &revisionCheckingModelRepo{model: model, persistedRevision: initialRevision}
	service := Service{
		ModelRepo:  repo,
		Authorizer: allowManagementAuthorizer{},
		Now:        func() time.Time { return now.Add(time.Minute) },
	}

	_, err = service.UpdateBasicInfo(context.Background(), modelcatalog.ActorContext{}, modelcatalog.UpdateBasicInfoDTO{
		Code: "SNAP-IV", Title: "SNAP-IV量表（26项）", Description: "请您根据孩子最近一段时间的情况作答",
		Category: "adhd", Stages: []string{"follow_up"}, ApplicableAges: []string{"school_child", "adolescent"},
		Reporters: []string{"parent", "teacher"}, Tags: []string{"注意缺陷", "多动冲动", "对立违抗"},
	})
	if err != nil {
		t.Fatalf("UpdateBasicInfo() error = %v", err)
	}
	if got, want := model.Revision(), initialRevision+1; got != want {
		t.Fatalf("revision = %d, want %d", got, want)
	}
	if got, want := model.Stages, []string{"follow_up"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stages = %#v, want %#v", got, want)
	}
	if got, want := model.ApplicableAges, []string{"school_child", "adolescent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("applicable ages = %#v, want %#v", got, want)
	}
	if got, want := model.Reporters, []string{"parent", "teacher"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reporters = %#v, want %#v", got, want)
	}
}

func TestUpdateBasicInfoForksPublishedModelToDraftWithOneRevision(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "MEDICAL-SCALE", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Before", Now: now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel() error = %v", err)
	}
	if err := model.MarkPublished(now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkPublished() error = %v", err)
	}
	publishedRevision := model.Revision()
	repo := &revisionCheckingModelRepo{model: model, persistedRevision: publishedRevision}
	service := Service{
		ModelRepo:  repo,
		Authorizer: allowManagementAuthorizer{},
		Now:        func() time.Time { return now.Add(2 * time.Minute) },
	}

	_, err = service.UpdateBasicInfo(context.Background(), modelcatalog.ActorContext{}, modelcatalog.UpdateBasicInfoDTO{
		Code: "MEDICAL-SCALE", Title: "After", Description: "updated description",
	})
	if err != nil {
		t.Fatalf("UpdateBasicInfo() error = %v", err)
	}
	if !model.IsDraft() {
		t.Fatalf("status = %s, want draft", model.Status)
	}
	if model.PublishedAt != nil {
		t.Fatalf("published_at = %v, want nil for the draft head", model.PublishedAt)
	}
	if got, want := model.Revision(), publishedRevision+1; got != want {
		t.Fatalf("revision = %d, want %d", got, want)
	}
}

func TestRestoreDraftFromPublishedCreatesEditableDraftWithoutChangingSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 5, 0, 0, 0, time.UTC)
	snapshot := &modelcatalogport.PublishedModel{
		Kind:                 domain.KindScale,
		SubKind:              domain.SubKindEmpty,
		Algorithm:            domain.AlgorithmScaleDefault,
		ProductChannel:       domain.ProductChannelMedicalScale,
		Code:                 "IPIP_BF50",
		Version:              "v17",
		Title:                "IPIP Big-Five",
		Description:          "人格探索",
		Category:             "personality",
		Stages:               []string{"adult"},
		ApplicableAges:       []string{"adult"},
		Reporters:            []string{"self"},
		Tags:                 []string{"人格"},
		QuestionnaireCode:    "IPIP-BF50-Q",
		QuestionnaireVersion: "1.0.0",
		PayloadFormat:        domain.PayloadFormatAssessmentScaleV1,
		Payload:              []byte(`{"scale_code":"IPIP_BF50"}`),
	}
	drafts := &restoredDraftRepo{models: map[string]*domain.AssessmentModel{}}
	service := Service{
		ModelRepo:  drafts,
		Published:  &publishedSnapshotRepo{snapshots: map[string]*modelcatalogport.PublishedModel{snapshotKey(snapshot.Kind, snapshot.Code): snapshot}},
		Authorizer: allowManagementAuthorizer{},
		Now:        func() time.Time { return now },
	}

	result, err := service.RestoreDraftFromPublished(context.Background(), modelcatalog.ActorContext{}, snapshot.Code)
	if err != nil {
		t.Fatalf("RestoreDraftFromPublished() error = %v", err)
	}
	if result.Status != string(domain.ModelStatusDraft) {
		t.Fatalf("result status = %q, want %q", result.Status, domain.ModelStatusDraft)
	}
	restored := drafts.models[snapshot.Code]
	if restored == nil {
		t.Fatal("restored draft was not created")
	}
	if !restored.IsDraft() || restored.PublishedAt != nil {
		t.Fatalf("restored model must be an editable draft: status=%q published_at=%v", restored.Status, restored.PublishedAt)
	}
	if got, want := restored.Revision(), int64(17); got != want {
		t.Fatalf("revision = %d, want %d", got, want)
	}
	if got, want := restored.Binding.QuestionnaireCode, snapshot.QuestionnaireCode; got != want {
		t.Fatalf("questionnaire code = %q, want %q", got, want)
	}
	if got, want := string(restored.Definition.Data), string(snapshot.Payload); got != want {
		t.Fatalf("definition payload = %q, want %q", got, want)
	}
	if snapshot.Category != "personality" {
		t.Fatalf("published snapshot category mutated to %q", snapshot.Category)
	}

	_, err = service.RestoreDraftFromPublished(context.Background(), modelcatalog.ActorContext{}, snapshot.Code)
	if err != nil {
		t.Fatalf("idempotent RestoreDraftFromPublished() error = %v", err)
	}
	if got, want := drafts.createCalls, 1; got != want {
		t.Fatalf("draft create calls = %d, want %d", got, want)
	}
}

type allowManagementAuthorizer struct{}

func (allowManagementAuthorizer) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}

type revisionCheckingModelRepo struct {
	model             *domain.AssessmentModel
	persistedRevision int64
}

func (*revisionCheckingModelRepo) Create(context.Context, *domain.AssessmentModel) error { return nil }

func (r *revisionCheckingModelRepo) Update(_ context.Context, model *domain.AssessmentModel) error {
	if got, want := model.Revision(), r.persistedRevision+1; got != want {
		return fmt.Errorf("optimistic-lock revision = %d, want %d", got, want)
	}
	r.model = model
	r.persistedRevision = model.Revision()
	return nil
}

func (r *revisionCheckingModelRepo) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model == nil || r.model.Code != code {
		return nil, domain.ErrNotFound
	}
	return r.model, nil
}

func (*revisionCheckingModelRepo) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (*revisionCheckingModelRepo) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (*revisionCheckingModelRepo) Delete(context.Context, string) error { return nil }

type restoredDraftRepo struct {
	models      map[string]*domain.AssessmentModel
	createCalls int
}

func (r *restoredDraftRepo) Create(_ context.Context, model *domain.AssessmentModel) error {
	r.createCalls++
	r.models[model.Code] = model
	return nil
}

func (r *restoredDraftRepo) Update(_ context.Context, model *domain.AssessmentModel) error {
	r.models[model.Code] = model
	return nil
}

func (r *restoredDraftRepo) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	model, ok := r.models[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return model, nil
}

func (*restoredDraftRepo) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}

func (*restoredDraftRepo) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}

func (*restoredDraftRepo) Delete(context.Context, string) error { return nil }

type publishedSnapshotRepo struct {
	snapshots map[string]*modelcatalogport.PublishedModel
}

func (*publishedSnapshotRepo) Save(context.Context, *modelcatalogport.PublishedModel) error {
	return nil
}

func (r *publishedSnapshotRepo) FindPublishedByModelCode(_ context.Context, kind domain.Kind, code string) (*modelcatalogport.PublishedModel, error) {
	snapshot, ok := r.snapshots[snapshotKey(kind, code)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return snapshot, nil
}

func (r *publishedSnapshotRepo) FindLatestPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*modelcatalogport.PublishedModel, error) {
	return r.FindPublishedByModelCode(ctx, kind, code)
}

func (r *publishedSnapshotRepo) FindPublishedByModelCodeVersion(ctx context.Context, kind domain.Kind, code, _ string) (*modelcatalogport.PublishedModel, error) {
	return r.FindPublishedByModelCode(ctx, kind, code)
}

func (*publishedSnapshotRepo) ListPublished(context.Context, modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	return nil, 0, nil
}

func (*publishedSnapshotRepo) DeletePublished(context.Context, domain.Kind, string) error { return nil }

func snapshotKey(kind domain.Kind, code string) string { return string(kind) + ":" + code }
