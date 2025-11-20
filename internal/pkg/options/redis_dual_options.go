package options

import (
	"github.com/spf13/pflag"
)

// RedisDualOptions defines options for dual redis instances (cache + store).
type RedisDualOptions struct {
	Cache *RedisInstanceOptions `json:"cache" mapstructure:"cache"`
	Store *RedisInstanceOptions `json:"store" mapstructure:"store"`
}

// RedisInstanceOptions defines options for a single redis instance.
type RedisInstanceOptions struct {
	Host          string `json:"host,omitempty"                     mapstructure:"host"`
	Port          int    `json:"port,omitempty"                     mapstructure:"port"`
	Username      string `json:"username,omitempty"                 mapstructure:"username"`
	Password      string `json:"-"                                  mapstructure:"password"`
	Database      int    `json:"database"                           mapstructure:"database"`
	MaxIdle       int    `json:"max-idle,omitempty"                 mapstructure:"max-idle"`
	MaxActive     int    `json:"max-active,omitempty"               mapstructure:"max-active"`
	Timeout       int    `json:"timeout,omitempty"                  mapstructure:"timeout"`
	EnableCluster bool   `json:"enable-cluster,omitempty"           mapstructure:"enable-cluster"`
	UseSSL        bool   `json:"use-ssl,omitempty"                  mapstructure:"use-ssl"`
	EnableLogging bool   `json:"enable-logging,omitempty"           mapstructure:"enable-logging"`
}

// NewRedisDualOptions create a `zero` value instance.
func NewRedisDualOptions() *RedisDualOptions {
	return &RedisDualOptions{
		Cache: &RedisInstanceOptions{
			Host:          "127.0.0.1",
			Port:          6379,
			Username:      "",
			Password:      "",
			Database:      0,
			MaxIdle:       50,
			MaxActive:     100,
			Timeout:       5,
			EnableCluster: false,
			UseSSL:        false,
			EnableLogging: true,
		},
		Store: &RedisInstanceOptions{
			Host:          "127.0.0.1",
			Port:          6379,
			Username:      "",
			Password:      "",
			Database:      1,
			MaxIdle:       50,
			MaxActive:     100,
			Timeout:       5,
			EnableCluster: false,
			UseSSL:        false,
			EnableLogging: true,
		},
	}
}

// Validate verifies flags passed to RedisDualOptions.
func (o *RedisDualOptions) Validate() []error {
	errs := []error{}

	return errs
}

// AddFlags adds flags related to dual redis storage for a specific APIServer to the specified FlagSet.
func (o *RedisDualOptions) AddFlags(fs *pflag.FlagSet) {
	// Cache Redis flags
	fs.StringVar(&o.Cache.Host, "redis.cache.host", o.Cache.Host, ""+
		"Cache Redis service host address.")

	fs.IntVar(&o.Cache.Port, "redis.cache.port", o.Cache.Port, ""+
		"Cache Redis service port.")

	fs.StringVar(&o.Cache.Username, "redis.cache.username", o.Cache.Username, ""+
		"Cache Redis username (Redis 6.0+ ACL).")

	fs.StringVar(&o.Cache.Password, "redis.cache.password", o.Cache.Password, ""+
		"Password for access to cache redis service.")

	fs.IntVar(&o.Cache.Database, "redis.cache.database", o.Cache.Database, ""+
		"Cache Redis database number.")

	fs.IntVar(&o.Cache.MaxIdle, "redis.cache.max-idle", o.Cache.MaxIdle, ""+
		"Maximum idle connections for cache redis.")

	fs.IntVar(&o.Cache.MaxActive, "redis.cache.max-active", o.Cache.MaxActive, ""+
		"Maximum active connections for cache redis.")

	fs.IntVar(&o.Cache.Timeout, "redis.cache.timeout", o.Cache.Timeout, ""+
		"Cache Redis connection timeout in seconds.")

	fs.BoolVar(&o.Cache.EnableCluster, "redis.cache.enable-cluster", o.Cache.EnableCluster, ""+
		"Enable cache redis cluster mode.")

	fs.BoolVar(&o.Cache.UseSSL, "redis.cache.use-ssl", o.Cache.UseSSL, ""+
		"Enable SSL for cache redis connection.")

	fs.BoolVar(&o.Cache.EnableLogging, "redis.cache.enable-logging", o.Cache.EnableLogging, ""+
		"Enable cache redis command logging.")

	// Store Redis flags
	fs.StringVar(&o.Store.Host, "redis.store.host", o.Store.Host, ""+
		"Store Redis service host address.")

	fs.IntVar(&o.Store.Port, "redis.store.port", o.Store.Port, ""+
		"Store Redis service port.")

	fs.StringVar(&o.Store.Username, "redis.store.username", o.Store.Username, ""+
		"Store Redis username (Redis 6.0+ ACL).")

	fs.StringVar(&o.Store.Password, "redis.store.password", o.Store.Password, ""+
		"Password for access to store redis service.")

	fs.IntVar(&o.Store.Database, "redis.store.database", o.Store.Database, ""+
		"Store Redis database number.")

	fs.IntVar(&o.Store.MaxIdle, "redis.store.max-idle", o.Store.MaxIdle, ""+
		"Maximum idle connections for store redis.")

	fs.IntVar(&o.Store.MaxActive, "redis.store.max-active", o.Store.MaxActive, ""+
		"Maximum active connections for store redis.")

	fs.IntVar(&o.Store.Timeout, "redis.store.timeout", o.Store.Timeout, ""+
		"Store Redis connection timeout in seconds.")

	fs.BoolVar(&o.Store.EnableCluster, "redis.store.enable-cluster", o.Store.EnableCluster, ""+
		"Enable store redis cluster mode.")

	fs.BoolVar(&o.Store.UseSSL, "redis.store.use-ssl", o.Store.UseSSL, ""+
		"Enable SSL for store redis connection.")

	fs.BoolVar(&o.Store.EnableLogging, "redis.store.enable-logging", o.Store.EnableLogging, ""+
		"Enable store redis command logging.")
}
