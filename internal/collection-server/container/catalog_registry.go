package container

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
)

// catalogKind 标识 BFF 侧一类 catalog L1 缓存实体。
type catalogKind int

const (
	catalogKindQuestionnaire catalogKind = iota
	catalogKindTypology
)

type catalogSpec struct {
	kind         string
	watcherLabel string
	config       func(*options.Options) *options.CatalogL1CacheOptions
	build        func(localCacheOptions) any
}

var catalogSpecs = map[catalogKind]catalogSpec{
	catalogKindQuestionnaire: {
		kind:         "questionnaire",
		watcherLabel: "questionnaire cache signal watcher",
		config:       questionnaireCatalogCfg,
		build: func(o localCacheOptions) any {
			return questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{
				TTL: o.TTL, MaxEntries: o.MaxEntries, TTLJitterRatio: o.TTLJitterRatio,
				OnHit: o.OnHit, OnMiss: o.OnMiss,
			})
		},
	},
	catalogKindTypology: {
		kind:         "typology",
		watcherLabel: "typology model cache signal watcher",
		config:       typologyCatalogCfg,
		build: func(o localCacheOptions) any {
			return typologymodel.NewLocalCatalogCache(typologymodel.LocalCatalogCacheOptions{
				TTL: o.TTL, MaxEntries: o.MaxEntries, TTLJitterRatio: o.TTLJitterRatio,
				OnHit: o.OnHit, OnMiss: o.OnMiss,
			})
		},
	},
}

func catalogL1Config(opts *options.Options, kind catalogKind) *options.CatalogL1CacheOptions {
	spec, ok := catalogSpecs[kind]
	if !ok || spec.config == nil {
		return nil
	}
	return spec.config(opts)
}

func catalogL1SingleflightEnabled(opts *options.Options, kind catalogKind) bool {
	if opts == nil {
		return false
	}
	return catalogSingleflightEnabled(catalogL1Config(opts, kind))
}

func newCatalogL1Cache(opts *options.Options, kind catalogKind) any {
	spec, ok := catalogSpecs[kind]
	if !ok || spec.build == nil {
		return nil
	}
	cfg := catalogCacheConfig(catalogL1Config(opts, kind))
	if cfg == nil {
		return nil
	}
	return spec.build(localCacheOptionsFromCatalog(spec.kind, cfg))
}

func questionnaireCatalogCfg(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.QuestionnaireCache == nil {
		return nil
	}
	return &opts.QuestionnaireCache.CatalogL1CacheOptions
}

func typologyCatalogCfg(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.TypologyCache == nil {
		return nil
	}
	return &opts.TypologyCache.CatalogL1CacheOptions
}
