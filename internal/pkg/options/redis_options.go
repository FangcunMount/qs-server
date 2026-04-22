package options

import (
	"github.com/spf13/pflag"
)

// RedisOptions defines options for redis database.
type RedisOptions struct {
	Host                  string   `json:"host,omitempty"                         mapstructure:"host"`
	Port                  int      `json:"port,omitempty"                         mapstructure:"port"`
	Addrs                 []string `json:"addrs,omitempty"                        mapstructure:"addrs"`
	Username              string   `json:"username,omitempty"                     mapstructure:"username"`
	Password              string   `json:"-"                                      mapstructure:"password"`
	Database              int      `json:"database"                               mapstructure:"database"`
	MaxIdle               int      `json:"max-idle,omitempty"                     mapstructure:"max-idle"`
	MaxActive             int      `json:"max-active,omitempty"                   mapstructure:"max-active"`
	Timeout               int      `json:"timeout,omitempty"                      mapstructure:"timeout"`
	MinIdleConns          int      `json:"min-idle-conns,omitempty"               mapstructure:"min-idle-conns"`
	PoolTimeout           int      `json:"pool-timeout,omitempty"                 mapstructure:"pool-timeout"`
	DialTimeout           int      `json:"dial-timeout,omitempty"                 mapstructure:"dial-timeout"`
	ReadTimeout           int      `json:"read-timeout,omitempty"                 mapstructure:"read-timeout"`
	WriteTimeout          int      `json:"write-timeout,omitempty"                mapstructure:"write-timeout"`
	EnableCluster         bool     `json:"enable-cluster,omitempty"               mapstructure:"enable-cluster"`
	UseSSL                bool     `json:"use-ssl,omitempty"                      mapstructure:"use-ssl"`
	SSLInsecureSkipVerify bool     `json:"ssl-insecure-skip-verify,omitempty"     mapstructure:"ssl-insecure-skip-verify"`
}

// NewRedisOptions create a `zero` value instance.
func NewRedisOptions() *RedisOptions {
	return &RedisOptions{
		Host:                  "127.0.0.1",
		Port:                  6379,
		Addrs:                 []string{},
		Username:              "",
		Password:              "",
		Database:              0,
		MaxIdle:               50,
		MaxActive:             100,
		Timeout:               5,
		MinIdleConns:          0,
		PoolTimeout:           5,
		DialTimeout:           5,
		ReadTimeout:           5,
		WriteTimeout:          5,
		EnableCluster:         false,
		UseSSL:                false,
		SSLInsecureSkipVerify: false,
	}
}

// Validate verifies flags passed to RedisOptions.
func (o *RedisOptions) Validate() []error {
	errs := []error{}

	return errs
}

// AddFlags adds flags related to redis storage for a specific APIServer to the specified FlagSet.
func (o *RedisOptions) AddFlags(fs *pflag.FlagSet) {
	addStringFlags(fs, []stringFlagSpec{
		{target: &o.Host, name: "redis.host", value: o.Host, usage: "" +
			"Redis service host address. If left blank, the following related redis options will be ignored."},
		{target: &o.Username, name: "redis.username", value: o.Username, usage: "" +
			"Redis username (Redis 6.0+ ACL)."},
		{target: &o.Password, name: "redis.password", value: o.Password, usage: "" +
			"Password for access to redis service."},
	})

	addStringSliceFlags(fs, []stringSliceFlagSpec{
		{target: &o.Addrs, name: "redis.addrs", value: o.Addrs, usage: "" +
			"Redis cluster addresses. If set, host and port will be ignored."},
	})

	addIntFlags(fs, []intFlagSpec{
		{target: &o.Port, name: "redis.port", value: o.Port, usage: "" +
			"Redis service port."},
		{target: &o.Database, name: "redis.database", value: o.Database, usage: "" +
			"Redis database number."},
		{target: &o.MaxIdle, name: "redis.max-idle", value: o.MaxIdle, usage: "" +
			"Maximum idle connections allowed to connect to redis."},
		{target: &o.MaxActive, name: "redis.max-active", value: o.MaxActive, usage: "" +
			"Maximum active connections allowed to connect to redis."},
		{target: &o.Timeout, name: "redis.timeout", value: o.Timeout, usage: "" +
			"Redis connection max idle time in seconds."},
		{target: &o.MinIdleConns, name: "redis.min-idle-conns", value: o.MinIdleConns, usage: "" +
			"Minimum number of idle connections to maintain."},
		{target: &o.PoolTimeout, name: "redis.pool-timeout", value: o.PoolTimeout, usage: "" +
			"Time in seconds to wait for a connection if the pool is exhausted."},
		{target: &o.DialTimeout, name: "redis.dial-timeout", value: o.DialTimeout, usage: "" +
			"Dial timeout in seconds."},
		{target: &o.ReadTimeout, name: "redis.read-timeout", value: o.ReadTimeout, usage: "" +
			"Read timeout in seconds."},
		{target: &o.WriteTimeout, name: "redis.write-timeout", value: o.WriteTimeout, usage: "" +
			"Write timeout in seconds."},
	})

	addBoolFlags(fs, []boolFlagSpec{
		{target: &o.EnableCluster, name: "redis.enable-cluster", value: o.EnableCluster, usage: "" +
			"Enable redis cluster mode."},
		{target: &o.UseSSL, name: "redis.use-ssl", value: o.UseSSL, usage: "" +
			"Enable SSL for redis connection."},
		{target: &o.SSLInsecureSkipVerify, name: "redis.ssl-insecure-skip-verify", value: o.SSLInsecureSkipVerify, usage: "" +
			"Skip SSL certificate verification."},
	})
}
