package cache

import (
	"context"

	catalogdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	catalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// InvalidatingPublishedModelRepository decorates published writes with cache invalidation.
type InvalidatingPublishedModelRepository struct {
	inner catalogport.PublishedModelRepository
	cache *CachedPublishedModelStore
}

func NewInvalidatingPublishedModelRepository(
	inner catalogport.PublishedModelRepository,
	cache *CachedPublishedModelStore,
) catalogport.PublishedModelRepository {
	if inner == nil {
		return nil
	}
	return &InvalidatingPublishedModelRepository{inner: inner, cache: cache}
}

func (r *InvalidatingPublishedModelRepository) Save(ctx context.Context, model *catalogport.PublishedModel) error {
	if err := r.inner.Save(ctx, model); err != nil {
		return err
	}
	r.invalidate(ctx, model)
	return nil
}

func (r *InvalidatingPublishedModelRepository) DeletePublished(ctx context.Context, kind catalogdomain.Kind, code string) error {
	existing, _ := r.inner.FindLatestPublishedByModelCode(ctx, kind, code)
	if err := r.inner.DeletePublished(ctx, kind, code); err != nil {
		return err
	}
	if existing != nil {
		r.invalidate(ctx, existing)
	}
	return nil
}

func (r *InvalidatingPublishedModelRepository) FindPublishedByModelCode(ctx context.Context, kind catalogdomain.Kind, code string) (*catalogport.PublishedModel, error) {
	return r.inner.FindPublishedByModelCode(ctx, kind, code)
}

func (r *InvalidatingPublishedModelRepository) FindLatestPublishedByModelCode(ctx context.Context, kind catalogdomain.Kind, code string) (*catalogport.PublishedModel, error) {
	return r.inner.FindLatestPublishedByModelCode(ctx, kind, code)
}

func (r *InvalidatingPublishedModelRepository) FindPublishedByModelCodeVersion(ctx context.Context, kind catalogdomain.Kind, code, version string) (*catalogport.PublishedModel, error) {
	return r.inner.FindPublishedByModelCodeVersion(ctx, kind, code, version)
}

func (r *InvalidatingPublishedModelRepository) ListPublished(ctx context.Context, filter catalogport.ListPublishedFilter) ([]*catalogport.PublishedModel, int64, error) {
	return r.inner.ListPublished(ctx, filter)
}

func (r *InvalidatingPublishedModelRepository) invalidate(ctx context.Context, model *catalogport.PublishedModel) {
	if r == nil || r.cache == nil || model == nil {
		return
	}
	r.cache.invalidatePublishedModel(ctx, model)
}
