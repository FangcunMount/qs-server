package grpc

import (
	"time"

	basemtls "github.com/FangcunMount/component-base/pkg/grpc/mtls"
)

// Config gRPC 服务器配置
type Config struct {
	// 基础配置
	BindAddress           string
	BindPort              int
	MaxMsgSize            int
	MaxConnectionAge      time.Duration
	MaxConnectionAgeGrace time.Duration

	// TLS 配置
	TLSCertFile string
	TLSKeyFile  string
	Insecure    bool // 是否使用不安全连接

	// mTLS 配置（使用 component-base 的配置）
	MTLS MTLSConfig

	// 应用层认证配置
	Auth AuthConfig

	// ACL 配置
	ACL ACLConfig

	// 审计配置
	Audit AuditConfig

	// 功能开关
	EnableReflection  bool
	EnableHealthCheck bool
	EnableMetrics     bool
}

// MTLSConfig mTLS 配置（映射到 component-base/pkg/grpc/mtls.Config）
type MTLSConfig struct {
	Enabled           bool
	CAFile            string
	AllowedCNs        []string
	AllowedOUs        []string
	AllowedDNSSANs    []string
	MinTLSVersion     string        // "1.2" 或 "1.3"
	EnableAutoReload  bool          // 证书热重载
	ReloadInterval    time.Duration // 重载检查间隔
	RequireClientCert bool
}

// AuthConfig 应用层认证配置
type AuthConfig struct {
	Enabled               bool
	EnableBearer          bool
	EnableHMAC            bool
	EnableAPIKey          bool
	HMACTimestampValidity time.Duration
	RequireIdentityMatch  bool // 要求凭证身份与 mTLS 身份一致
}

// ACLConfig ACL 配置
type ACLConfig struct {
	Enabled       bool
	ConfigFile    string // ACL 规则文件路径
	DefaultPolicy string // "allow" | "deny"
}

// AuditConfig 审计配置
type AuditConfig struct {
	Enabled    bool
	OutputPath string
}

// NewConfig 创建默认配置
func NewConfig() *Config {
	return &Config{
		BindAddress:           "0.0.0.0",
		BindPort:              9090,
		MaxMsgSize:            4 * 1024 * 1024,
		MaxConnectionAge:      2 * time.Hour,
		MaxConnectionAgeGrace: 10 * time.Second,
		Insecure:              true,

		MTLS: MTLSConfig{
			Enabled:           false,
			RequireClientCert: false,
			MinTLSVersion:     "1.2",
			EnableAutoReload:  false,
			ReloadInterval:    5 * time.Minute,
		},

		Auth: AuthConfig{
			Enabled:               false,
			EnableBearer:          false,
			EnableHMAC:            false,
			EnableAPIKey:          false,
			HMACTimestampValidity: 5 * time.Minute,
			RequireIdentityMatch:  false,
		},

		ACL: ACLConfig{
			Enabled:       false,
			DefaultPolicy: "deny",
		},

		Audit: AuditConfig{
			Enabled: false,
		},

		EnableReflection:  true,
		EnableHealthCheck: true,
		EnableMetrics:     false,
	}
}

// ToBaseMTLSConfig 转换为 component-base 的 mTLS 配置
func (c *MTLSConfig) ToBaseMTLSConfig(certFile, keyFile string) *basemtls.Config {
	cfg := basemtls.DefaultConfig()
	cfg.CertFile = certFile
	cfg.KeyFile = keyFile
	cfg.CAFile = c.CAFile
	cfg.RequireClientCert = c.RequireClientCert
	cfg.AllowedCNs = c.AllowedCNs
	cfg.AllowedOUs = c.AllowedOUs
	cfg.AllowedDNSSANs = c.AllowedDNSSANs
	cfg.MinVersion = parseTLSVersion(c.MinTLSVersion)
	cfg.EnableAutoReload = c.EnableAutoReload
	cfg.ReloadInterval = c.ReloadInterval
	return cfg
}

// parseTLSVersion 解析 TLS 版本
func parseTLSVersion(version string) uint16 {
	switch version {
	case "1.3":
		return 0x0304 // tls.VersionTLS13
	case "1.2":
		return 0x0303 // tls.VersionTLS12
	default:
		return 0x0303 // 默认 TLS 1.2
	}
}

// CompletedConfig 完成的配置
type CompletedConfig struct {
	*Config
}

// Complete 填充默认值
func (c *Config) Complete() CompletedConfig {
	if c.BindAddress == "" {
		c.BindAddress = "0.0.0.0"
	}
	if c.BindPort == 0 {
		c.BindPort = 9090
	}
	if c.MaxMsgSize == 0 {
		c.MaxMsgSize = 4 * 1024 * 1024
	}
	if c.MaxConnectionAge == 0 {
		c.MaxConnectionAge = 2 * time.Hour
	}
	if c.MaxConnectionAgeGrace == 0 {
		c.MaxConnectionAgeGrace = 10 * time.Second
	}
	if c.MTLS.MinTLSVersion == "" {
		c.MTLS.MinTLSVersion = "1.2"
	}
	if c.MTLS.ReloadInterval == 0 {
		c.MTLS.ReloadInterval = 5 * time.Minute
	}
	if c.Auth.HMACTimestampValidity == 0 {
		c.Auth.HMACTimestampValidity = 5 * time.Minute
	}
	if c.ACL.DefaultPolicy == "" {
		c.ACL.DefaultPolicy = "deny"
	}
	return CompletedConfig{c}
}

// New 创建服务器
func (c CompletedConfig) New() (*Server, error) {
	return NewServer(c.Config)
}
