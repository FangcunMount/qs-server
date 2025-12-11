package options

import (
	"time"

	"github.com/spf13/pflag"
)

// MongoDBOptions defines options for mongodb database.
// 与 component-base/pkg/database.MongoConfig 保持一致的结构
type MongoDBOptions struct {
	// 分离的连接参数（推荐使用，便于通过环境变量配置）
	Host     string `json:"host,omitempty"     mapstructure:"host"`     // 主机地址，格式: host:port
	Username string `json:"username,omitempty" mapstructure:"username"` // 用户名
	Password string `json:"-"                  mapstructure:"password"` // 密码（不输出到JSON）
	Database string `json:"database,omitempty" mapstructure:"database"` // 数据库名

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
		Host:                     "127.0.0.1:27017",
		Username:                 "",
		Password:                 "",
		Database:                 "",
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

	return errs
}

// AddFlags adds flags related to mongodb storage for a specific APIServer to the specified FlagSet.
func (o *MongoDBOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Host, "mongodb.host", o.Host, ""+
		"MongoDB service host address (format: host:port).")

	fs.StringVar(&o.Username, "mongodb.username", o.Username, ""+
		"Username for access to MongoDB service.")

	fs.StringVar(&o.Password, "mongodb.password", o.Password, ""+
		"Password for access to MongoDB service.")

	fs.StringVar(&o.Database, "mongodb.database", o.Database, ""+
		"Database name for the server to use.")

	fs.BoolVar(&o.UseSSL, "mongodb.use-ssl", o.UseSSL, ""+
		"Enable SSL for mongodb connection.")

	fs.BoolVar(&o.SSLInsecureSkipVerify, "mongodb.ssl-insecure-skip-verify", o.SSLInsecureSkipVerify, ""+
		"Skip SSL certificate verification for mongodb.")

	fs.BoolVar(&o.SSLAllowInvalidHostnames, "mongodb.ssl-allow-invalid-hostnames", o.SSLAllowInvalidHostnames, ""+
		"Allow invalid hostnames in SSL certificates for mongodb.")

	fs.StringVar(&o.SSLCAFile, "mongodb.ssl-ca-file", o.SSLCAFile, ""+
		"Path to SSL CA certificate file for mongodb.")

	fs.StringVar(&o.SSLPEMKeyfile, "mongodb.ssl-pem-keyfile", o.SSLPEMKeyfile, ""+
		"Path to SSL PEM key file for mongodb.")

	fs.BoolVar(&o.EnableLogger, "mongodb.enable-logger", o.EnableLogger, ""+
		"Enable MongoDB command logging.")

	fs.DurationVar(&o.SlowThreshold, "mongodb.slow-threshold", o.SlowThreshold, ""+
		"Slow query threshold for mongodb (e.g., 200ms).")

	// 详细日志配置（component-base v0.4.1+ 已支持）
	fs.BoolVar(&o.LogCommandDetail, "mongodb.log-command-detail", o.LogCommandDetail, ""+
		"Enable detailed command logging (includes query statements, sensitive data will be sanitized).")

	fs.BoolVar(&o.LogReplyDetail, "mongodb.log-reply-detail", o.LogReplyDetail, ""+
		"Enable detailed reply logging (may increase log size significantly).")

	fs.BoolVar(&o.LogStarted, "mongodb.log-started", o.LogStarted, ""+
		"Enable logging of command start events (increases log volume, use for debugging only).")
}
