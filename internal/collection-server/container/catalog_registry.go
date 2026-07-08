package container

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
)

// catalogKind 标识 BFF 侧一类 catalog L1 缓存实体。
type catalogKind int

const (
	catalogKindQuestionnaire catalogKind = iota
	catalogKindScale
	catalogKindPersonality
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
	catalogKindScale: {
		kind:         "scale",
		watcherLabel: "scale cache signal watcher",
		config:       scaleCatalogCfg,
		build: func(o localCacheOptions) any {
			return scale.NewLocalCatalogCache(scale.LocalCatalogCacheOptions{
				TTL: o.TTL, MaxEntries: o.MaxEntries, TTLJitterRatio: o.TTLJitterRatio,
				OnHit: o.OnHit, OnMiss: o.OnMiss,
			})
		},
	},
	catalogKindPersonality: {
		kind:         "personality",
		watcherLabel: "personality model cache signal watcher",
		config:       personalityCatalogCfg,
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

func scaleCatalogCfg(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.ScaleCache == nil {
		return nil
	}
	return &opts.ScaleCache.CatalogL1CacheOptions
}

func personalityCatalogCfg(opts *options.Options) *options.CatalogL1CacheOptions {
	if opts == nil || opts.PersonalityCache == nil {
		return nil
	}
	return &opts.PersonalityCache.CatalogL1CacheOptions
}
