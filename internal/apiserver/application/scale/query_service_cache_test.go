package scale

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"
	"time"

	domainscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
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
	service := NewQueryService(repo, repo, nil, cache, nil)

	got, err := service.ListPublished(context.Background(), ListScalesDTO{Page: 1, PageSize: 10})
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
		pages: map[int][]*domainscale.MedicalScale{
			1: {newScaleCacheQueryScale(t, "SCALE_DB", "DB Scale", domainscale.StatusPublished)},
		},
	}
	cache := &publishedScaleListCacheStub{hit: false}
	service := NewQueryService(repo, repo, nil, cache, nil)

	got, err := service.ListPublished(context.Background(), ListScalesDTO{Page: 1, PageSize: 10})
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

	repo := &scaleCacheQueryRepo{
		byCode: map[string]*domainscale.MedicalScale{
			"SCALE_DRAFT": newScaleCacheQueryScale(t, "SCALE_DRAFT", "Draft Scale", domainscale.StatusDraft),
		},
	}
	cache := &publishedScaleListCacheStub{rebuildErr: stderrors.New("cache unavailable")}
	service := NewLifecycleService(repo, nil, nil, cache)

	if err := service.Delete(context.Background(), "SCALE_DRAFT"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if repo.removeCalls.Load() != 1 {
		t.Fatalf("Remove calls = %d, want 1", repo.removeCalls.Load())
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
	pages            map[int][]*domainscale.MedicalScale
	byCode           map[string]*domainscale.MedicalScale
	findSummaryCalls atomic.Int32
	countCalls       atomic.Int32
	removeCalls      atomic.Int32
}

func (r *scaleCacheQueryRepo) Create(context.Context, *domainscale.MedicalScale) error {
	return nil
}

func (r *scaleCacheQueryRepo) FindByCode(_ context.Context, code string) (*domainscale.MedicalScale, error) {
	if item, ok := r.byCode[code]; ok {
		return item, nil
	}
	return nil, domainscale.ErrNotFound
}

func (r *scaleCacheQueryRepo) FindByQuestionnaireCode(context.Context, string) (*domainscale.MedicalScale, error) {
	return nil, domainscale.ErrNotFound
}

func (r *scaleCacheQueryRepo) ListScales(_ context.Context, _ scalereadmodel.ScaleFilter, page scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	r.findSummaryCalls.Add(1)
	return scaleCacheQueryRows(r.pages[page.Page]), nil
}

func (r *scaleCacheQueryRepo) CountScales(context.Context, scalereadmodel.ScaleFilter) (int64, error) {
	r.countCalls.Add(1)
	return r.count, nil
}

func (r *scaleCacheQueryRepo) Update(context.Context, *domainscale.MedicalScale) error {
	return nil
}

func (r *scaleCacheQueryRepo) Remove(context.Context, string) error {
	r.removeCalls.Add(1)
	return nil
}

func (r *scaleCacheQueryRepo) ExistsByCode(context.Context, string) (bool, error) {
	return false, nil
}

func scaleCacheQueryRows(items []*domainscale.MedicalScale) []scalereadmodel.ScaleSummaryRow {
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

func newScaleCacheQueryScale(t *testing.T, code, title string, status domainscale.Status) *domainscale.MedicalScale {
	t.Helper()

	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	scale, err := domainscale.NewMedicalScale(
		meta.NewCode(code),
		title,
		domainscale.WithDescription("description"),
		domainscale.WithQuestionnaire(meta.NewCode("Q_"+code), "v1"),
		domainscale.WithStatus(status),
		domainscale.WithCategory(domainscale.CategoryADHD),
		domainscale.WithCreatedBy(meta.ID(101)),
		domainscale.WithUpdatedBy(meta.ID(102)),
		domainscale.WithCreatedAt(now),
		domainscale.WithUpdatedAt(now),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return scale
}
