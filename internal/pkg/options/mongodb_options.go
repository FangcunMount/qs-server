package options

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
)

// MongoDBOptions defines options for mongodb database.
// 与 component-base/pkg/database.MongoConfig 保持一致的结构
type MongoDBOptions struct {
	// 直接连接 URL（优先级最高）
	URL string `json:"url,omitempty" mapstructure:"url"`

	// 分离的连接参数（推荐使用，便于通过环境变量配置）
	Host     string `json:"host,omitempty"     mapstructure:"host"`     // 主机地址，格式: host:port
	Username string `json:"username,omitempty" mapstructure:"username"` // 用户名
	Password string `json:"-"                  mapstructure:"password"` // 密码（不输出到JSON）
	Database string `json:"database,omitempty" mapstructure:"database"` // 数据库名

	// 事务 / 拓扑配置
	ReplicaSet       string `json:"replica-set,omitempty"      mapstructure:"replica-set"`
	DirectConnection bool   `json:"direct-connection,omitempty" mapstructure:"direct-connection"`

	// 连接池配置。0 表示保留 MongoDB Go Driver 默认值，便于直接 URL 继续自行携带参数。
	MinPoolSize     uint64        `json:"min-pool-size,omitempty"     mapstructure:"min-pool-size"`
	MaxPoolSize     uint64        `json:"max-pool-size,omitempty"     mapstructure:"max-pool-size"`
	MaxConnecting   uint64        `json:"max-connecting,omitempty"    mapstructure:"max-connecting"`
	MaxConnIdleTime time.Duration `json:"max-conn-idle-time,omitempty" mapstructure:"max-conn-idle-time"`

	// SSL 配置
	UseSSL                   bool   `json:"use-ssl,omitempty"                  mapstructure:"use-ssl"`
	SSLInsecureSkipVerify    bool   `json:"ssl-insecure-skip-verify,omitempty" mapstructure:"ssl-insecure-skip-verify"`
	SSLAllowInvalidHostnames bool   `json:"ssl-allow-invalid-hostnames,omitempty" mapstructure:"ssl-allow-invalid-hostnames"`
	SSLCAFile                string `json:"ssl-ca-file,omitempty"              mapstructure:"ssl-ca-file"`
	SSLPEMKeyfile            string `json:"ssl-pem-keyfile,omitempty"          mapstructure:"ssl-pem-keyfile"`

	// 日志配置
	EnableLogger  bool          `json:"enable-logger,omitempty"  mapstructure:"enable-logger"`  // 是否启用日志
	SlowThreshold time.Duration `json:"slow-threshold,omitempty" mapstructure:"slow-threshold"` // 慢查询阈值

	// 详细日志配置（component-base v0.4.1+ 已支持）
	LogCommandDetail bool `json:"log-command-detail,omitempty" mapstructure:"log-command-detail"` // 是否记录命令详情（查询语句）
	LogReplyDetail   bool `json:"log-reply-detail,omitempty"   mapstructure:"log-reply-detail"`   // 是否记录响应详情
	LogStarted       bool `json:"log-started,omitempty"        mapstructure:"log-started"`        // 是否记录命令开始
}

// NewMongoDBOptions create a `zero` value instance.
func NewMongoDBOptions() *MongoDBOptions {
	return &MongoDBOptions{
		URL:                      "",
		Host:                     "127.0.0.1:27017",
		Username:                 "",
		Password:                 "",
		Database:                 "",
		ReplicaSet:               "",
		DirectConnection:         false,
		MinPoolSize:              0,
		MaxPoolSize:              0,
		MaxConnecting:            0,
		MaxConnIdleTime:          0,
		UseSSL:                   false,
		SSLInsecureSkipVerify:    false,
		SSLAllowInvalidHostnames: false,
		SSLCAFile:                "",
		SSLPEMKeyfile:            "",
		EnableLogger:             true,                   // 默认启用 MongoDB 日志
		SlowThreshold:            200 * time.Millisecond, // 默认慢查询阈值 200ms
		// 详细日志配置（开发环境可以启用，生产环境按需配置）
		LogCommandDetail: true,  // 默认启用查询详情（类似 GORM 的 SQL 日志，敏感信息会自动脱敏）
		LogReplyDetail:   false, // 默认不记录响应详情（避免日志过大）
		LogStarted:       false, // 默认不记录命令开始（减少日志量）
	}
}

// Validate verifies flags passed to MongoDBOptions.
func (o *MongoDBOptions) Validate() []error {
	errs := []error{}
	if o == nil {
		return errs
	}
	if o.MaxPoolSize > 0 && o.MinPoolSize > o.MaxPoolSize {
		errs = append(errs, fmt.Errorf("mongodb.min-pool-size (%d) must be <= max-pool-size (%d)", o.MinPoolSize, o.MaxPoolSize))
	}
	if o.MaxPoolSize > 0 && o.MaxConnecting > o.MaxPoolSize {
		errs = append(errs, fmt.Errorf("mongodb.max-connecting (%d) must be <= max-pool-size (%d)", o.MaxConnecting, o.MaxPoolSize))
	}
	if o.MaxConnecting > 100 {
		errs = append(errs, fmt.Errorf("mongodb.max-connecting (%d) must be <= 100", o.MaxConnecting))
	}
	if o.MaxConnIdleTime < 0 {
		errs = append(errs, fmt.Errorf("mongodb.max-conn-idle-time must not be negative"))
	}
	if o.MaxConnIdleTime > 0 && o.MaxConnIdleTime < time.Millisecond {
		errs = append(errs, fmt.Errorf("mongodb.max-conn-idle-time must be at least 1ms when configured"))
	}

	return errs
}

// AddFlags adds flags related to mongodb storage for a specific APIServer to the specified FlagSet.
func (o *MongoDBOptions) AddFlags(fs *pflag.FlagSet) {
	addMongoDBConnectionFlags(fs, o)
	addMongoDBTLSFlags(fs, o)
	addMongoDBLoggingFlags(fs, o)
}

func addMongoDBConnectionFlags(fs *pflag.FlagSet, o *MongoDBOptions) {
	addStringFlags(fs, []stringFlagSpec{
		{target: &o.URL, name: "mongodb.url", value: o.URL, usage: "" +
			"Full MongoDB connection URI. If set, it takes precedence over separated host/credential fields."},
		{target: &o.Host, name: "mongodb.host", value: o.Host, usage: "" +
			"MongoDB service host address (format: host:port)."},
		{target: &o.Username, name: "mongodb.username", value: o.Username, usage: "" +
			"Username for access to MongoDB service."},
		{target: &o.Password, name: "mongodb.password", value: o.Password, usage: "" +
			"Password for access to MongoDB service."},
		{target: &o.Database, name: "mongodb.database", value: o.Database, usage: "" +
			"Database name for the server to use."},
		{target: &o.ReplicaSet, name: "mongodb.replica-set", value: o.ReplicaSet, usage: "" +
			"Replica set name for MongoDB transactions (for example: rs0)."},
	})
	addBoolFlag(fs, &o.DirectConnection, "mongodb.direct-connection", o.DirectConnection, ""+
		"Force directConnection=true for single-node replica set deployments.")
	fs.Uint64Var(&o.MinPoolSize, "mongodb.min-pool-size", o.MinPoolSize,
		"Minimum MongoDB connections maintained per server. Zero keeps the driver default.")
	fs.Uint64Var(&o.MaxPoolSize, "mongodb.max-pool-size", o.MaxPoolSize,
		"Maximum MongoDB connections per server. Zero keeps the driver default.")
	fs.Uint64Var(&o.MaxConnecting, "mongodb.max-connecting", o.MaxConnecting,
		"Maximum MongoDB connections established concurrently. Zero keeps the driver default.")
	addDurationFlag(fs, &o.MaxConnIdleTime, "mongodb.max-conn-idle-time", o.MaxConnIdleTime,
		"Maximum idle time for a pooled MongoDB connection. Zero keeps the driver default.")
}

func addMongoDBTLSFlags(fs *pflag.FlagSet, o *MongoDBOptions) {
	addBoolFlags(fs, []boolFlagSpec{
		{target: &o.UseSSL, name: "mongodb.use-ssl", value: o.UseSSL, usage: "" +
			"Enable SSL for mongodb connection."},
		{target: &o.SSLInsecureSkipVerify, name: "mongodb.ssl-insecure-skip-verify", value: o.SSLInsecureSkipVerify, usage: "" +
			"Skip SSL certificate verification for mongodb."},
		{target: &o.SSLAllowInvalidHostnames, name: "mongodb.ssl-allow-invalid-hostnames", value: o.SSLAllowInvalidHostnames, usage: "" +
			"Allow invalid hostnames in SSL certificates for mongodb."},
	})
	addStringFlags(fs, []stringFlagSpec{
		{target: &o.SSLCAFile, name: "mongodb.ssl-ca-file", value: o.SSLCAFile, usage: "" +
			"Path to SSL CA certificate file for mongodb."},
		{target: &o.SSLPEMKeyfile, name: "mongodb.ssl-pem-keyfile", value: o.SSLPEMKeyfile, usage: "" +
			"Path to SSL PEM key file for mongodb."},
	})
}

func addMongoDBLoggingFlags(fs *pflag.FlagSet, o *MongoDBOptions) {
	addBoolFlags(fs, []boolFlagSpec{
		{target: &o.EnableLogger, name: "mongodb.enable-logger", value: o.EnableLogger, usage: "" +
			"Enable MongoDB command logging."},
		{target: &o.LogCommandDetail, name: "mongodb.log-command-detail", value: o.LogCommandDetail, usage: "" +
			"Enable detailed command logging (includes query statements, sensitive data will be sanitized)."},
		{target: &o.LogReplyDetail, name: "mongodb.log-reply-detail", value: o.LogReplyDetail, usage: "" +
			"Enable detailed reply logging (may increase log size significantly)."},
		{target: &o.LogStarted, name: "mongodb.log-started", value: o.LogStarted, usage: "" +
			"Enable logging of command start events (increases log volume, use for debugging only)."},
	})
	addDurationFlag(fs, &o.SlowThreshold, "mongodb.slow-threshold", o.SlowThreshold, ""+
		"Slow query threshold for mongodb (e.g., 200ms).")
}
