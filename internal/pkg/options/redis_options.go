package options

import (
	"github.com/spf13/pflag"
)

// RedisOptions defines options for redis database.
type RedisOptions struct {
	Host                  string   `json:"host,omitempty"                         mapstructure:"host"`
	Port                  int      `json:"port,omitempty"                         mapstructure:"port"`
	Addrs                 []string `json:"addrs,omitempty"                        mapstructure:"addrs"`
	Password              string   `json:"-"                                      mapstructure:"password"`
	Database              int      `json:"database"                               mapstructure:"database"`
	MaxIdle               int      `json:"max-idle,omitempty"                     mapstructure:"max-idle"`
	MaxActive             int      `json:"max-active,omitempty"                   mapstructure:"max-active"`
	Timeout               int      `json:"timeout,omitempty"                      mapstructure:"timeout"`
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
		Password:              "",
		Database:              0,
		MaxIdle:               50,
		MaxActive:             100,
		Timeout:               5,
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
	fs.StringVar(&o.Host, "redis.host", o.Host, ""+
		"Redis service host address. If left blank, the following related redis options will be ignored.")

	fs.IntVar(&o.Port, "redis.port", o.Port, ""+
		"Redis service port.")

	fs.StringSliceVar(&o.Addrs, "redis.addrs", o.Addrs, ""+
		"Redis cluster addresses. If set, host and port will be ignored.")

	fs.StringVar(&o.Password, "redis.password", o.Password, ""+
		"Password for access to redis service.")

	fs.IntVar(&o.Database, "redis.database", o.Database, ""+
		"Redis database number.")

	fs.IntVar(&o.MaxIdle, "redis.max-idle", o.MaxIdle, ""+
		"Maximum idle connections allowed to connect to redis.")

	fs.IntVar(&o.MaxActive, "redis.max-active", o.MaxActive, ""+
		"Maximum active connections allowed to connect to redis.")

	fs.IntVar(&o.Timeout, "redis.timeout", o.Timeout, ""+
		"Redis connection timeout in seconds.")

	fs.BoolVar(&o.EnableCluster, "redis.enable-cluster", o.EnableCluster, ""+
		"Enable redis cluster mode.")

	fs.BoolVar(&o.UseSSL, "redis.use-ssl", o.UseSSL, ""+
		"Enable SSL for redis connection.")

	fs.BoolVar(&o.SSLInsecureSkipVerify, "redis.ssl-insecure-skip-verify", o.SSLInsecureSkipVerify, ""+
		"Skip SSL certificate verification.")
}
