package options

import (
	"fmt"
	"net"
	"time"

	"github.com/spf13/pflag"
)

// GRPCOptions GRPC 服务器配置选项
type GRPCOptions struct {
	BindAddress string `json:"bind_address" mapstructure:"bind-address"` // 绑定地址
	BindPort    int    `json:"bind_port"    mapstructure:"bind-port"`    // 绑定端口
	HealthzPort int    `json:"healthz_port" mapstructure:"healthz-port"` // 健康检查端口

	// TLS 配置
	Insecure    bool   `json:"insecure"       mapstructure:"insecure"`      // 是否不安全模式（无 TLS）
	TLSCertFile string `json:"tls_cert_file"  mapstructure:"tls-cert-file"` // TLS 证书文件
	TLSKeyFile  string `json:"tls_key_file"   mapstructure:"tls-key-file"`  // TLS 密钥文件

	// 消息和连接配置
	MaxMsgSize            int           `json:"max_msg_size"              mapstructure:"max-msg-size"`             // 最大消息大小（字节）
	MaxConnectionAge      time.Duration `json:"max_connection_age"        mapstructure:"max-connection-age"`       // 最大连接存活时间
	MaxConnectionAgeGrace time.Duration `json:"max_connection_age_grace"  mapstructure:"max-connection-age-grace"` // 连接关闭宽限期

	// mTLS 配置
	MTLS *MTLSOptions `json:"mtls" mapstructure:"mtls"`

	// 认证配置
	Auth *AuthOptions `json:"auth" mapstructure:"auth"`

	// ACL 配置
	ACL *ACLOptions `json:"acl" mapstructure:"acl"`

	// 审计配置
	Audit *AuditOptions `json:"audit" mapstructure:"audit"`

	// 功能开关
	EnableReflection  bool `json:"enable_reflection"   mapstructure:"enable-reflection"`   // 启用反射
	EnableHealthCheck bool `json:"enable_health_check" mapstructure:"enable-health-check"` // 启用健康检查
}

// MTLSOptions mTLS 配置选项
type MTLSOptions struct {
	Enabled           bool     `json:"enabled"              mapstructure:"enabled"`             // 是否启用 mTLS
	CAFile            string   `json:"ca_file"              mapstructure:"ca-file"`             // CA 证书文件
	RequireClientCert bool     `json:"require_client_cert"  mapstructure:"require-client-cert"` // 是否要求客户端证书
	AllowedCNs        []string `json:"allowed_cns"          mapstructure:"allowed-cns"`         // 允许的客户端 CN 列表
	AllowedOUs        []string `json:"allowed_ous"          mapstructure:"allowed-ous"`         // 允许的客户端 OU 列表
	MinTLSVersion     string   `json:"min_tls_version"      mapstructure:"min-tls-version"`     // 最小 TLS 版本（1.2 或 1.3）
}

// AuthOptions 认证配置选项
type AuthOptions struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"` // 是否启用认证
}

// ACLOptions ACL 配置选项
type ACLOptions struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"` // 是否启用 ACL
}

// AuditOptions 审计配置选项
type AuditOptions struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"` // 是否启用审计
}

// NewGRPCOptions 创建默认的 GRPC 配置选项
func NewGRPCOptions() *GRPCOptions {
	return &GRPCOptions{
		BindAddress:           "127.0.0.1",
		BindPort:              9090,
		HealthzPort:           9091,
		Insecure:              true,              // 默认不安全模式（开发环境）
		MaxMsgSize:            4194304,           // 4MB
		MaxConnectionAge:      120 * time.Second, // 120s
		MaxConnectionAgeGrace: 20 * time.Second,  // 20s
		EnableReflection:      true,              // 默认启用反射
		EnableHealthCheck:     true,              // 默认启用健康检查
		MTLS: &MTLSOptions{
			Enabled:           false,
			RequireClientCert: false,
			MinTLSVersion:     "1.2",
		},
		Auth: &AuthOptions{
			Enabled: false,
		},
		ACL: &ACLOptions{
			Enabled: false,
		},
		Audit: &AuditOptions{
			Enabled: false,
		},
	}
}

// Validate 验证GRPCOptions
func (s *GRPCOptions) Validate() []error {
	var errors []error

	if s.BindPort < 0 || s.BindPort > 65535 {
		errors = append(
			errors,
			fmt.Errorf(
				"--grpc.bind-port %v must be between 0 and 65535, inclusive. 0 for turning off insecure (HTTP) port",
				s.BindPort,
			),
		)
	}

	// 验证 TLS 配置
	if !s.Insecure {
		if s.TLSCertFile == "" {
			errors = append(errors, fmt.Errorf("tls-cert-file is required when insecure is false"))
		}
		if s.TLSKeyFile == "" {
			errors = append(errors, fmt.Errorf("tls-key-file is required when insecure is false"))
		}
	}

	// 验证 mTLS 配置
	if s.MTLS != nil && s.MTLS.Enabled {
		if s.Insecure {
			errors = append(errors, fmt.Errorf("mTLS cannot be enabled when insecure mode is on"))
		}
		if s.MTLS.CAFile == "" {
			errors = append(errors, fmt.Errorf("mtls.ca-file is required when mTLS is enabled"))
		}
		if s.MTLS.MinTLSVersion != "" && s.MTLS.MinTLSVersion != "1.2" && s.MTLS.MinTLSVersion != "1.3" {
			errors = append(errors, fmt.Errorf("mtls.min-tls-version must be 1.2 or 1.3"))
		}
	}

	return errors
}

// AddFlags 添加命令行参数
func (s *GRPCOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.BindAddress, "grpc.bind-address", s.BindAddress, ""+
		"The IP address on which to serve the --grpc.bind-port(set to 0.0.0.0 for all IPv4 interfaces and :: for all IPv6 interfaces).")

	fs.IntVar(&s.BindPort, "grpc.bind-port", s.BindPort, ""+
		"The port on which to serve unsecured, unauthenticated grpc access. It is assumed "+
		"that firewall rules are set up such that this port is not reachable from outside of "+
		"the deployed machine and that port 443 on the iam public address is proxied to this "+
		"port. This is performed by nginx in the default setup. Set to zero to disable.")

	fs.IntVar(&s.HealthzPort, "grpc.healthz-port", s.HealthzPort, ""+
		"The port on which to serve grpc health check.")
}

// ApplyTo 应用配置到服务器
func (s *GRPCOptions) ApplyTo(c *GRPCConfig) error {
	c.Addr = net.JoinHostPort(s.BindAddress, fmt.Sprintf("%d", s.BindPort))
	c.HealthzAddr = net.JoinHostPort(s.BindAddress, fmt.Sprintf("%d", s.HealthzPort))

	return nil
}
