package ruleset

import (
	"context"
	"fmt"

	modelcatalogcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"go.mongodb.org/mongo-driver/mongo"
)

type runtimePublishedStore interface {
	port.PublishedModelReader
	port.PublishedModelLister
}

// RuntimePublishedCatalog reads only published_assessment_models v2 at runtime.
type RuntimePublishedCatalog struct {
	store runtimePublishedStore
}

var (
	_ port.Catalog              = (*RuntimePublishedCatalog)(nil)
	_ port.PublishedModelReader = (*RuntimePublishedCatalog)(nil)
	_ port.PublishedModelLister = (*RuntimePublishedCatalog)(nil)
)

// NewRuntimePublishedCatalog builds the production runtime catalog backed by v2 published snapshots.
func NewRuntimePublishedCatalog(
	db *mongo.Database,
	mongoOpts mongoBase.BaseRepositoryOptions,
	cacheCfg PublishedModelCacheConfig,
) (port.Catalog, error) {
	if db == nil {
		return nil, fmt.Errorf("mongo database is nil")
	}
	v2 := mongomodelcatalog.NewRepository(db, mongoOpts)
	store := aminfra.NewPublishedStore(v2)
	var publishedStore runtimePublishedStore = store
	if cacheCfg.enabled() {
		publishedStore = modelcatalogcache.NewCachedPublishedModelStore(store, cacheCfg.Redis, cacheCfg.Builder, cacheCfg.Policy, cacheCfg.Observer)
	}
	return &RuntimePublishedCatalog{store: publishedStore}, nil
}

// NewRuntimePublishedCatalogWithStore wires a runtime catalog for tests.
func NewRuntimePublishedCatalogWithStore(store runtimePublishedStore) *RuntimePublishedCatalog {
	return &RuntimePublishedCatalog{store: store}
}

func (c *RuntimePublishedCatalog) ResolveByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (port.Ref, bool, error) {
	if c == nil || c.store == nil {
		return port.Ref{}, false, nil
	}
	snapshot, err := c.store.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		if domain.IsNotFound(err) {
			return port.Ref{}, false, nil
		}
		return port.Ref{}, false, err
	}
	return port.RefFromPublished(snapshot), true, nil
}

func (c *RuntimePublishedCatalog) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if c == nil || c.store == nil {
		return nil, domain.ErrNotFound
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	return c.store.GetPublishedModelByRef(ctx, ref)
}

func (c *RuntimePublishedCatalog) FindPublishedModelByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*port.PublishedModel, error) {
	if c == nil || c.store == nil {
		return nil, domain.ErrNotFound
	}
	return c.store.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (c *RuntimePublishedCatalog) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	if c == nil || c.store == nil {
		return nil, domain.ErrNotFound
	}
	return c.store.FindPublishedModelByCode(ctx, kind, code)
}

func (c *RuntimePublishedCatalog) ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	if c == nil || c.store == nil {
		return nil, 0, domain.ErrNotFound
	}
	return c.store.ListPublishedModels(ctx, filter)
}
