package query

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/lifecycle"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScaleQueryServiceListPublishedUsesCachePort(t *testing.T) {
	t.Parallel()

	repo := &scaleCacheQueryRepo{}
	cache := &publishedScaleListCacheStub{
		hit: true,
		page: &scalelistcache.Page{
			Total: 1,
			Items: []scalelistcache.Summary{{
				Code:  "SCALE_CACHE",
				Title: "Cached Scale",
			}},
		},
	}
	service := NewQueryService(repo, nil, cache, nil)

	got, err := service.ListPublished(context.Background(), shared.ListScalesDTO{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListPublished() error = %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.Items[0].Code != "SCALE_CACHE" {
		t.Fatalf("ListPublished() = %#v, want cached SCALE_CACHE", got)
	}
	if repo.findSummaryCalls.Load() != 0 || repo.countCalls.Load() != 0 {
		t.Fatalf("repo calls = find:%d count:%d, want no repository fallback", repo.findSummaryCalls.Load(), repo.countCalls.Load())
	}
}

func TestScaleQueryServiceListPublishedFallsBackWhenCacheMisses(t *testing.T) {
	t.Parallel()

	repo := &scaleCacheQueryRepo{
		count: 1,
		pages: map[int][]*scaledefinition.MedicalScale{
			1: {newScaleCacheQueryScale(t, "SCALE_DB", "DB Scale", scaledefinition.StatusPublished)},
		},
	}
	cache := &publishedScaleListCacheStub{hit: false}
	service := NewQueryService(repo, nil, cache, nil)

	got, err := service.ListPublished(context.Background(), shared.ListScalesDTO{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListPublished() error = %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.Items[0].Code != "SCALE_DB" {
		t.Fatalf("ListPublished() = %#v, want DB fallback SCALE_DB", got)
	}
	if repo.findSummaryCalls.Load() != 1 || repo.countCalls.Load() != 1 {
		t.Fatalf("repo calls = find:%d count:%d, want one repository fallback", repo.findSummaryCalls.Load(), repo.countCalls.Load())
	}
}

func TestScaleLifecycleDeleteIgnoresListCacheRebuildFailure(t *testing.T) {
	t.Parallel()

	model := newScaleCacheQueryAssessmentModel(t, "SCALE_DRAFT", "Draft Scale")
	modelRepo := &scaleCacheDeleteModelRepo{model: model}
	publishedRepo := &scaleCacheDeletePublishedRepo{}
	cache := &publishedScaleListCacheStub{rebuildErr: stderrors.New("cache unavailable")}
	service := lifecycle.NewService(
		nil,
		nil,
		cache,
		lifecycle.WithAssessmentModelRepository(modelRepo),
		lifecycle.WithPublishedModelRepository(publishedRepo),
		lifecycle.WithPublicationPublisher(assessmentstore.NewPublicationPublisher(modelRepo, publishedRepo)),
	)

	if err := service.Delete(context.Background(), "SCALE_DRAFT"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if modelRepo.deleteCalls.Load() != 1 {
		t.Fatalf("Delete calls = %d, want 1", modelRepo.deleteCalls.Load())
	}
	if cache.rebuildCalls.Load() != 1 {
		t.Fatalf("Rebuild calls = %d, want 1", cache.rebuildCalls.Load())
	}
}

type publishedScaleListCacheStub struct {
	hit          bool
	page         *scalelistcache.Page
	rebuildErr   error
	rebuildCalls atomic.Int32
}

func (c *publishedScaleListCacheStub) Rebuild(context.Context) error {
	c.rebuildCalls.Add(1)
	return c.rebuildErr
}

func (c *publishedScaleListCacheStub) GetPage(context.Context, int, int) (*scalelistcache.Page, bool) {
	return c.page, c.hit
}

type scaleCacheQueryRepo struct {
	count            int64
	pages            map[int][]*scaledefinition.MedicalScale
	byCode           map[string]*scaledefinition.MedicalScale
	findSummaryCalls atomic.Int32
	countCalls       atomic.Int32
	removeCalls      atomic.Int32
}

func (r *scaleCacheQueryRepo) Create(context.Context, *scaledefinition.MedicalScale) error {
	return nil
}

func (r *scaleCacheQueryRepo) CreatePublishedSnapshot(context.Context, *scaledefinition.MedicalScale, bool) error {
	return nil
}

func (r *scaleCacheQueryRepo) FindByCode(_ context.Context, code string) (*scaledefinition.MedicalScale, error) {
	if item, ok := r.byCode[code]; ok {
		return item, nil
	}
	return nil, scaledefinition.ErrNotFound
}

func (r *scaleCacheQueryRepo) FindPublishedByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error) {
	return r.FindByCode(ctx, code)
}

func (r *scaleCacheQueryRepo) FindByCodeVersion(ctx context.Context, code, _ string) (*scaledefinition.MedicalScale, error) {
	return r.FindByCode(ctx, code)
}

func (r *scaleCacheQueryRepo) FindByQuestionnaireCode(context.Context, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}

func (r *scaleCacheQueryRepo) FindPublishedByQuestionnaireCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error) {
	return r.FindByQuestionnaireCode(ctx, code)
}

func (r *scaleCacheQueryRepo) FindByQuestionnaireRef(context.Context, string, string) (*scaledefinition.MedicalScale, error) {
	return nil, scaledefinition.ErrNotFound
}

func (r *scaleCacheQueryRepo) ListScales(_ context.Context, _ scalereadmodel.ScaleFilter, page scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	r.findSummaryCalls.Add(1)
	return scaleCacheQueryRows(r.pages[page.Page]), nil
}

func (r *scaleCacheQueryRepo) CountScales(context.Context, scalereadmodel.ScaleFilter) (int64, error) {
	r.countCalls.Add(1)
	return r.count, nil
}

func (r *scaleCacheQueryRepo) Update(context.Context, *scaledefinition.MedicalScale) error {
	return nil
}

func (r *scaleCacheQueryRepo) Remove(context.Context, string) error {
	r.removeCalls.Add(1)
	return nil
}

func (r *scaleCacheQueryRepo) SetActivePublishedVersion(context.Context, string, string) error {
	return nil
}

func (r *scaleCacheQueryRepo) ClearActivePublishedVersion(context.Context, string) error {
	return nil
}

func (r *scaleCacheQueryRepo) ExistsByCode(context.Context, string) (bool, error) {
	return false, nil
}

func scaleCacheQueryRows(items []*scaledefinition.MedicalScale) []scalereadmodel.ScaleSummaryRow {
	rows := make([]scalereadmodel.ScaleSummaryRow, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		rows = append(rows, scalereadmodel.ScaleSummaryRow{
			Code:              item.GetCode().String(),
			Title:             item.GetTitle(),
			Description:       item.GetDescription(),
			Category:          item.GetCategory().String(),
			QuestionnaireCode: item.GetQuestionnaireCode().String(),
			Status:            item.GetStatus().String(),
			CreatedBy:         item.GetCreatedBy(),
			CreatedAt:         item.GetCreatedAt(),
			UpdatedBy:         item.GetUpdatedBy(),
			UpdatedAt:         item.GetUpdatedAt(),
		})
	}
	return rows
}

func newScaleCacheQueryScale(t *testing.T, code, title string, status scaledefinition.Status) *scaledefinition.MedicalScale {
	t.Helper()

	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode(code),
		title,
		scaledefinition.WithDescription("description"),
		scaledefinition.WithQuestionnaire(meta.NewCode("Q_"+code), "v1"),
		scaledefinition.WithStatus(status),
		scaledefinition.WithCategory(scaledefinition.CategoryADHD),
		scaledefinition.WithCreatedBy(meta.ID(101)),
		scaledefinition.WithUpdatedBy(meta.ID(102)),
		scaledefinition.WithCreatedAt(now),
		scaledefinition.WithUpdatedAt(now),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return scale
}

func newScaleCacheQueryAssessmentModel(t *testing.T, code, title string) *domain.AssessmentModel {
	t.Helper()
	scale := newScaleCacheQueryScale(t, code, title, scaledefinition.StatusDraft)
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale() error = %v", err)
	}
	model.Code = code
	return model
}

type scaleCacheDeleteModelRepo struct {
	model       *domain.AssessmentModel
	deleteCalls atomic.Int32
}

func (r *scaleCacheDeleteModelRepo) Create(context.Context, *domain.AssessmentModel) error {
	return nil
}
func (r *scaleCacheDeleteModelRepo) Update(context.Context, *domain.AssessmentModel) error {
	return nil
}
func (r *scaleCacheDeleteModelRepo) FindByCode(_ context.Context, code string) (*domain.AssessmentModel, error) {
	if r.model != nil && r.model.Code == code {
		return r.model, nil
	}
	return nil, domain.ErrNotFound
}
func (r *scaleCacheDeleteModelRepo) FindByQuestionnaireCode(context.Context, domain.Kind, string) (*domain.AssessmentModel, error) {
	return nil, domain.ErrNotFound
}
func (r *scaleCacheDeleteModelRepo) List(context.Context, modelcatalogport.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	return nil, 0, nil
}
func (r *scaleCacheDeleteModelRepo) Delete(context.Context, string) error {
	r.deleteCalls.Add(1)
	return nil
}

type scaleCacheDeletePublishedRepo struct{}

func (r *scaleCacheDeletePublishedRepo) Save(context.Context, *modelcatalogport.PublishedModel) error {
	return nil
}
func (r *scaleCacheDeletePublishedRepo) FindPublishedByModelCode(context.Context, domain.Kind, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}
func (r *scaleCacheDeletePublishedRepo) FindLatestPublishedByModelCode(context.Context, domain.Kind, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}
func (r *scaleCacheDeletePublishedRepo) FindPublishedByModelCodeVersion(context.Context, domain.Kind, string, string) (*modelcatalogport.PublishedModel, error) {
	return nil, domain.ErrNotFound
}
func (r *scaleCacheDeletePublishedRepo) ListPublished(context.Context, modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	return nil, 0, nil
}
func (r *scaleCacheDeletePublishedRepo) DeletePublished(context.Context, domain.Kind, string) error {
	return nil
}

var (
	_ modelcatalogport.ModelRepository          = (*scaleCacheDeleteModelRepo)(nil)
	_ modelcatalogport.PublishedModelRepository = (*scaleCacheDeletePublishedRepo)(nil)
)
