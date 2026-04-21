package options

import "github.com/spf13/pflag"

// RedisRuntimeOptions defines qs-server runtime routing above Foundation redis config.
type RedisRuntimeOptions struct {
	Namespace string                              `json:"namespace" mapstructure:"namespace"`
	Families  map[string]*RedisRuntimeFamilyRoute `json:"families" mapstructure:"families"`
}

// RedisRuntimeFamilyRoute defines logical family routing to a redis profile/keyspace.
type RedisRuntimeFamilyRoute struct {
	RedisProfile         string `json:"redis_profile" mapstructure:"redis_profile"`
	NamespaceSuffix      string `json:"namespace_suffix" mapstructure:"namespace_suffix"`
	AllowFallbackDefault *bool  `json:"allow_fallback_default,omitempty" mapstructure:"allow_fallback_default"`
	AllowWarmup          *bool  `json:"allow_warmup,omitempty" mapstructure:"allow_warmup"`
}

// NewRedisRuntimeOptions creates a zero-value runtime routing config.
func NewRedisRuntimeOptions() *RedisRuntimeOptions {
	return &RedisRuntimeOptions{
		Namespace: "",
		Families:  map[string]*RedisRuntimeFamilyRoute{},
	}
}

// AddFlags adds the shared namespace flag. Family-level routing is config-file driven.
func (o *RedisRuntimeOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	fs.StringVar(&o.Namespace, "redis_runtime.namespace", o.Namespace,
		"Optional Redis runtime namespace prefix shared by logical Redis families.")
}
