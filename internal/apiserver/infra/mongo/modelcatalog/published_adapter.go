package modelcatalog

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedModelRepoAdapter implements port.PublishedModelRepository on top of the v2 Mongo repository.
type PublishedModelRepoAdapter struct {
	inner *Repository
}

var _ port.PublishedModelRepository = (*PublishedModelRepoAdapter)(nil)

func NewPublishedModelRepoAdapter(inner *Repository) *PublishedModelRepoAdapter {
	return &PublishedModelRepoAdapter{inner: inner}
}

func (a *PublishedModelRepoAdapter) Save(ctx context.Context, snapshot *domain.PublishedModelSnapshot) error {
	if a == nil || a.inner == nil {
		return domain.ErrNotFound
	}
	return a.inner.UpsertPublishedModel(ctx, snapshot)
}

func (a *PublishedModelRepoAdapter) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	return a.FindLatestPublishedByModelCode(ctx, kind, code)
}

func (a *PublishedModelRepoAdapter) FindLatestPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.PublishedModelSnapshot, error) {
	if a == nil || a.inner == nil {
		return nil, domain.ErrNotFound
	}
	return a.inner.FindLatestPublishedModelByModelCode(ctx, kind, code)
}

func (a *PublishedModelRepoAdapter) FindPublishedByModelCodeVersion(ctx context.Context, kind domain.Kind, code, version string) (*domain.PublishedModelSnapshot, error) {
	if a == nil || a.inner == nil {
		return nil, domain.ErrNotFound
	}
	published, err := a.inner.GetPublishedModelByRef(ctx, port.Ref{Kind: kind, Code: code, Version: version})
	if err != nil {
		return nil, err
	}
	return published, nil
}

func (a *PublishedModelRepoAdapter) ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.PublishedModelSnapshot, int64, error) {
	if a == nil || a.inner == nil {
		return nil, 0, domain.ErrNotFound
	}
	return a.inner.ListPublishedModels(ctx, filter)
}

func (a *PublishedModelRepoAdapter) DeletePublished(ctx context.Context, kind domain.Kind, code string) error {
	if a == nil || a.inner == nil || code == "" {
		return domain.ErrNotFound
	}
	now := time.Now()
	_, err := a.inner.Collection().UpdateMany(ctx, publishedFilter(bson.M{
		"model_kind": string(kind),
		"model_code": code,
	}), bson.M{"$set": bson.M{
		"deleted_at": now,
		"updated_at": now,
		"status":     "unpublished",
	}})
	return err
}
