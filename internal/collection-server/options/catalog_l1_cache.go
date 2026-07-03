package options

import (
	"fmt"

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

// ScaleCacheOptions 量表目录 BFF 进程内 L1 缓存。
type ScaleCacheOptions struct {
	CatalogL1CacheOptions `mapstructure:",squash"`
}

// PersonalityCacheOptions 人格模型目录 BFF 进程内 L1 缓存。
type PersonalityCacheOptions struct {
	CatalogL1CacheOptions `mapstructure:",squash"`
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

// NewScaleCacheOptions 创建默认量表目录 L1 缓存配置。
func NewScaleCacheOptions() *ScaleCacheOptions {
	return &ScaleCacheOptions{CatalogL1CacheOptions: NewCatalogL1CacheOptions()}
}

// NewPersonalityCacheOptions 创建默认人格模型目录 L1 缓存配置。
func NewPersonalityCacheOptions() *PersonalityCacheOptions {
	return &PersonalityCacheOptions{CatalogL1CacheOptions: NewCatalogL1CacheOptions()}
}

func (o *CatalogL1CacheOptions) addFlags(fs *pflag.FlagSet, prefix, label string) {
	if o == nil || fs == nil {
		return
	}
	fs.BoolVar(&o.Enabled, prefix+".enabled", o.Enabled, fmt.Sprintf("Enable in-process L1 cache for %s.", label))
	fs.IntVar(&o.TTLSeconds, prefix+".ttl-seconds", o.TTLSeconds, fmt.Sprintf("TTL for %s L1 cache in seconds.", label))
	fs.Float64Var(&o.TTLJitterRatio, prefix+".ttl-jitter-ratio", o.TTLJitterRatio, fmt.Sprintf("TTL jitter ratio (0-1) for %s L1 cache.", label))
	fs.IntVar(&o.MaxEntries, prefix+".max-entries", o.MaxEntries, fmt.Sprintf("Maximum %s entries in L1 cache.", label))
	fs.BoolVar(&o.Singleflight, prefix+".singleflight", o.Singleflight, fmt.Sprintf("Coalesce concurrent %s cache misses.", label))
	fs.BoolVar(&o.SignalEvictEnabled, prefix+".signal-evict-enabled", o.SignalEvictEnabled, fmt.Sprintf("Subscribe Redis signal to evict %s L1 entries.", label))
}

func (q *QuestionnaireCacheOptions) AddFlags(fs *pflag.FlagSet) {
	q.CatalogL1CacheOptions.addFlags(fs, "questionnaire_cache", "published questionnaire detail")
}

func (s *ScaleCacheOptions) AddFlags(fs *pflag.FlagSet) {
	s.CatalogL1CacheOptions.addFlags(fs, "scale_cache", "scale catalog reads")
}

func (p *PersonalityCacheOptions) AddFlags(fs *pflag.FlagSet) {
	p.CatalogL1CacheOptions.addFlags(fs, "personality_cache", "personality model catalog reads")
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

func validateScaleCacheOptions(opts *ScaleCacheOptions) []error {
	if opts == nil {
		return nil
	}
	return validateCatalogL1CacheOptions(&opts.CatalogL1CacheOptions, "scale_cache")
}

func validatePersonalityCacheOptions(opts *PersonalityCacheOptions) []error {
	if opts == nil {
		return nil
	}
	return validateCatalogL1CacheOptions(&opts.CatalogL1CacheOptions, "personality_cache")
}
