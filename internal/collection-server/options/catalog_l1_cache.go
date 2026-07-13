package options

import (
	"fmt"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/spf13/pflag"
)

// CatalogL1CacheOptions BFF 进程内目录 L1 缓存通用配置。
type CatalogL1CacheOptions struct {
	Enabled            bool    `json:"enabled" mapstructure:"enabled"`
	TTLSeconds         int     `json:"ttl_seconds" mapstructure:"ttl_seconds"`
	TTLJitterRatio     float64 `json:"ttl_jitter_ratio" mapstructure:"ttl_jitter_ratio"`
	MaxEntries         int     `json:"max_entries" mapstructure:"max_entries"`
	Singleflight       bool    `json:"singleflight" mapstructure:"singleflight"`
	SignalEvictEnabled bool    `json:"signal_evict_enabled" mapstructure:"signal_evict_enabled"`
}

// QuestionnaireCacheOptions 已发布问卷详情 BFF 进程内 L1 缓存。
type QuestionnaireCacheOptions struct {
	CatalogL1CacheOptions `mapstructure:",squash"`
}

// TypologyCacheOptions 类型学模型目录 BFF 进程内 L1 缓存。
type TypologyCacheOptions struct {
	CatalogL1CacheOptions `mapstructure:",squash"`
}

type CacheOptions struct {
	Capabilities *CacheCapabilities `json:"capabilities" mapstructure:"capabilities"`
}

type CacheCapabilities struct {
	Catalog      *CatalogCacheCapabilities           `json:"catalog" mapstructure:"catalog"`
	ReportStatus *genericoptions.ReportStatusOptions `json:"report_status" mapstructure:"report_status"`
}

type CatalogCacheCapabilities struct {
	Questionnaire *QuestionnaireCacheOptions `json:"questionnaire" mapstructure:"questionnaire"`
	Typology      *TypologyCacheOptions      `json:"typology" mapstructure:"typology"`
}

func NewCacheOptions() *CacheOptions {
	return &CacheOptions{Capabilities: &CacheCapabilities{
		Catalog: &CatalogCacheCapabilities{
			Questionnaire: NewQuestionnaireCacheOptions(),
			Typology:      NewTypologyCacheOptions(),
		},
		ReportStatus: genericoptions.NewReportStatusOptions(),
	}}
}

// NewCatalogL1CacheOptions 创建默认目录 L1 缓存配置。
func NewCatalogL1CacheOptions() CatalogL1CacheOptions {
	return CatalogL1CacheOptions{
		Enabled:            false,
		TTLSeconds:         180,
		MaxEntries:         256,
		Singleflight:       true,
		SignalEvictEnabled: true,
	}
}

// NewQuestionnaireCacheOptions 创建默认问卷详情 L1 缓存配置。
func NewQuestionnaireCacheOptions() *QuestionnaireCacheOptions {
	return &QuestionnaireCacheOptions{CatalogL1CacheOptions: NewCatalogL1CacheOptions()}
}

// NewTypologyCacheOptions 创建默认类型学模型目录 L1 缓存配置。
func NewTypologyCacheOptions() *TypologyCacheOptions {
	return &TypologyCacheOptions{CatalogL1CacheOptions: NewCatalogL1CacheOptions()}
}

func (o *CatalogL1CacheOptions) addFlags(fs *pflag.FlagSet, prefix, label string) {
	if o == nil || fs == nil {
		return
	}
	fs.BoolVar(&o.Enabled, prefix+".enabled", o.Enabled, fmt.Sprintf("Enable in-process L1 cache for %s.", label))
	fs.IntVar(&o.TTLSeconds, prefix+".ttl_seconds", o.TTLSeconds, fmt.Sprintf("TTL for %s L1 cache in seconds.", label))
	fs.Float64Var(&o.TTLJitterRatio, prefix+".ttl_jitter_ratio", o.TTLJitterRatio, fmt.Sprintf("TTL jitter ratio (0-1) for %s L1 cache.", label))
	fs.IntVar(&o.MaxEntries, prefix+".max_entries", o.MaxEntries, fmt.Sprintf("Maximum %s entries in L1 cache.", label))
	fs.BoolVar(&o.Singleflight, prefix+".singleflight", o.Singleflight, fmt.Sprintf("Coalesce concurrent %s cache misses.", label))
	fs.BoolVar(&o.SignalEvictEnabled, prefix+".signal_evict_enabled", o.SignalEvictEnabled, fmt.Sprintf("Subscribe Redis signal to evict %s L1 entries.", label))
}

func (q *QuestionnaireCacheOptions) AddFlags(fs *pflag.FlagSet) {
	q.addFlags(fs, "cache.capabilities.catalog.questionnaire", "published questionnaire detail")
}

func (p *TypologyCacheOptions) AddFlags(fs *pflag.FlagSet) {
	p.addFlags(fs, "cache.capabilities.catalog.typology", "typology model catalog reads")
}

func validateCatalogL1CacheOptions(opts *CatalogL1CacheOptions, name string) []error {
	if opts == nil || !opts.Enabled {
		return nil
	}
	var errs []error
	if opts.TTLSeconds <= 0 {
		errs = append(errs, fmt.Errorf("%s.ttl_seconds must be greater than 0 when enabled", name))
	}
	if opts.MaxEntries <= 0 {
		errs = append(errs, fmt.Errorf("%s.max_entries must be greater than 0 when enabled", name))
	}
	if opts.TTLJitterRatio < 0 || opts.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("%s.ttl_jitter_ratio must be between 0 and 1", name))
	}
	return errs
}

func validateQuestionnaireCacheOptions(opts *QuestionnaireCacheOptions) []error {
	if opts == nil {
		return nil
	}
	return validateCatalogL1CacheOptions(&opts.CatalogL1CacheOptions, "questionnaire_cache")
}

func validateTypologyCacheOptions(opts *TypologyCacheOptions) []error {
	if opts == nil {
		return nil
	}
	return validateCatalogL1CacheOptions(&opts.CatalogL1CacheOptions, "typology_cache")
}
