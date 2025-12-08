package options

import (
	"time"

	"github.com/spf13/pflag"
)

// IAMOptions IAM 集成配置
type IAMOptions struct {
	// 功能开关
	Enabled     bool `json:"enabled"       mapstructure:"enabled"`
	GRPCEnabled bool `json:"grpc-enabled"  mapstructure:"grpc-enabled"`
	JWKSEnabled bool `json:"jwks-enabled"  mapstructure:"jwks-enabled"`

	// gRPC 配置
	GRPC *IAMGRPCOptions `json:"grpc" mapstructure:"grpc"`

	// JWT 验证配置
	JWT *IAMJWTOptions `json:"jwt" mapstructure:"jwt"`

	// JWKS 配置
	JWKS *IAMJWKSOptions `json:"jwks" mapstructure:"jwks"`

	// 服务间认证配置
	ServiceAuth *IAMServiceAuthOptions `json:"service-auth" mapstructure:"service-auth"`

	// 用户信息缓存配置
	UserCache *IAMCacheOptions `json:"user-cache" mapstructure:"user-cache"`

	// 监护关系缓存配置
	GuardianshipCache *IAMCacheOptions `json:"guardianship-cache" mapstructure:"guardianship-cache"`
}

// IAMGRPCOptions IAM gRPC 连接配置
type IAMGRPCOptions struct {
	Address  string         `json:"address"    mapstructure:"address"`
	Timeout  time.Duration  `json:"timeout"    mapstructure:"timeout"`
	RetryMax int            `json:"retry-max"  mapstructure:"retry-max"`
	TLS      *IAMTLSOptions `json:"tls"        mapstructure:"tls"`
}

// IAMTLSOptions mTLS 证书配置
type IAMTLSOptions struct {
	Enabled  bool   `json:"enabled"   mapstructure:"enabled"`
	CAFile   string `json:"ca-file"   mapstructure:"ca-file"`
	CertFile string `json:"cert-file" mapstructure:"cert-file"`
	KeyFile  string `json:"key-file"  mapstructure:"key-file"`
}

// IAMJWTOptions JWT 验证配置
type IAMJWTOptions struct {
	Issuer         string        `json:"issuer"           mapstructure:"issuer"`
	Audience       []string      `json:"audience"         mapstructure:"audience"`
	Algorithms     []string      `json:"algorithms"       mapstructure:"algorithms"`
	ClockSkew      time.Duration `json:"clock-skew"       mapstructure:"clock-skew"`
	RequiredClaims []string      `json:"required-claims"  mapstructure:"required-claims"`
}

// IAMJWKSOptions JWKS 配置
type IAMJWKSOptions struct {
	URL             string        `json:"url"               mapstructure:"url"`
	RefreshInterval time.Duration `json:"refresh-interval"  mapstructure:"refresh-interval"`
	CacheTTL        time.Duration `json:"cache-ttl"         mapstructure:"cache-ttl"`
	FetchStrategies []string      `json:"fetch-strategies"  mapstructure:"fetch-strategies"`
}

// IAMServiceAuthOptions 服务间认证配置
type IAMServiceAuthOptions struct {
	ServiceID      string        `json:"service-id"       mapstructure:"service-id"`
	TargetAudience []string      `json:"target-audience"  mapstructure:"target-audience"`
	TokenTTL       time.Duration `json:"token-ttl"        mapstructure:"token-ttl"`
	RefreshBefore  time.Duration `json:"refresh-before"   mapstructure:"refresh-before"`
}

// IAMCacheOptions 缓存配置
type IAMCacheOptions struct {
	Enabled bool          `json:"enabled"  mapstructure:"enabled"`
	TTL     time.Duration `json:"ttl"      mapstructure:"ttl"`
	MaxSize int           `json:"max-size" mapstructure:"max-size"`
}

// NewIAMOptions 创建默认的 IAM 配置
func NewIAMOptions() *IAMOptions {
	return &IAMOptions{
		Enabled:     false, // 默认关闭，需要明确启用
		GRPCEnabled: true,
		JWKSEnabled: true,

		GRPC: &IAMGRPCOptions{
			Address:  "127.0.0.1:9090",
			Timeout:  5 * time.Second,
			RetryMax: 3,
			TLS: &IAMTLSOptions{
				Enabled:  true,
				CAFile:   "./configs/cert/grpc/ca-chain.crt",
				CertFile: "./configs/cert/grpc/qs-client.crt",
				KeyFile:  "./configs/cert/grpc/qs-client.key",
			},
		},

		JWT: &IAMJWTOptions{
			Issuer:         "https://iam.example.com",
			Audience:       []string{"qs"},
			Algorithms:     []string{"RS256", "ES256"},
			ClockSkew:      60 * time.Second,
			RequiredClaims: []string{"user_id"},
		},

		JWKS: &IAMJWKSOptions{
			URL:             "https://iam.example.com/.well-known/jwks.json",
			RefreshInterval: 5 * time.Minute,
			CacheTTL:        30 * time.Minute,
			FetchStrategies: []string{"http", "grpc", "cache"},
		},

		ServiceAuth: &IAMServiceAuthOptions{
			ServiceID:      "qs-service",
			TargetAudience: []string{"iam-service"},
			TokenTTL:       1 * time.Hour,
			RefreshBefore:  5 * time.Minute,
		},

		UserCache: &IAMCacheOptions{
			Enabled: true,
			TTL:     5 * time.Minute,
			MaxSize: 10000,
		},

		GuardianshipCache: &IAMCacheOptions{
			Enabled: true,
			TTL:     10 * time.Minute,
			MaxSize: 50000,
		},
	}
}

// Validate 验证配置
func (o *IAMOptions) Validate() []error {
	var errs []error

	if !o.Enabled {
		return nil // 未启用，跳过验证
	}

	// 验证 gRPC 配置
	if o.GRPCEnabled {
		if o.GRPC.Address == "" {
			errs = append(errs, ErrIAMGRPCAddressRequired)
		}
		if o.GRPC.TLS != nil && o.GRPC.TLS.Enabled {
			if o.GRPC.TLS.CAFile == "" {
				errs = append(errs, ErrIAMTLSCAFileRequired)
			}
			if o.GRPC.TLS.CertFile == "" {
				errs = append(errs, ErrIAMTLSCertFileRequired)
			}
			if o.GRPC.TLS.KeyFile == "" {
				errs = append(errs, ErrIAMTLSKeyFileRequired)
			}
		}
	}

	// 验证 JWT 配置
	if o.JWKSEnabled {
		if o.JWT.Issuer == "" {
			errs = append(errs, ErrIAMJWTIssuerRequired)
		}
		if len(o.JWT.Audience) == 0 {
			errs = append(errs, ErrIAMJWTAudienceRequired)
		}
		if o.JWKS.URL == "" {
			errs = append(errs, ErrIAMJWKSURLRequired)
		}
	}

	return errs
}

// AddFlags 添加命令行参数
func (o *IAMOptions) AddFlags(fs *pflag.FlagSet) {
	// 功能开关
	fs.BoolVar(&o.Enabled, "iam.enabled", o.Enabled,
		"Enable IAM integration (灰度发布开关)")
	fs.BoolVar(&o.GRPCEnabled, "iam.grpc-enabled", o.GRPCEnabled,
		"Enable IAM gRPC calls")
	fs.BoolVar(&o.JWKSEnabled, "iam.jwks-enabled", o.JWKSEnabled,
		"Enable JWKS local token verification")

	// gRPC 配置
	fs.StringVar(&o.GRPC.Address, "iam.grpc.address", o.GRPC.Address,
		"IAM gRPC server address (host:port)")
	fs.DurationVar(&o.GRPC.Timeout, "iam.grpc.timeout", o.GRPC.Timeout,
		"IAM gRPC request timeout")
	fs.IntVar(&o.GRPC.RetryMax, "iam.grpc.retry-max", o.GRPC.RetryMax,
		"IAM gRPC max retry count")

	// mTLS 配置
	fs.BoolVar(&o.GRPC.TLS.Enabled, "iam.grpc.tls.enabled", o.GRPC.TLS.Enabled,
		"Enable mTLS for IAM gRPC connection")
	fs.StringVar(&o.GRPC.TLS.CAFile, "iam.grpc.tls.ca-file", o.GRPC.TLS.CAFile,
		"IAM gRPC CA certificate file path")
	fs.StringVar(&o.GRPC.TLS.CertFile, "iam.grpc.tls.cert-file", o.GRPC.TLS.CertFile,
		"IAM gRPC client certificate file path")
	fs.StringVar(&o.GRPC.TLS.KeyFile, "iam.grpc.tls.key-file", o.GRPC.TLS.KeyFile,
		"IAM gRPC client private key file path")

	// JWT 配置
	fs.StringVar(&o.JWT.Issuer, "iam.jwt.issuer", o.JWT.Issuer,
		"JWT token issuer")
	fs.StringSliceVar(&o.JWT.Audience, "iam.jwt.audience", o.JWT.Audience,
		"JWT allowed audience list")
	fs.DurationVar(&o.JWT.ClockSkew, "iam.jwt.clock-skew", o.JWT.ClockSkew,
		"JWT clock skew tolerance")

	// JWKS 配置
	fs.StringVar(&o.JWKS.URL, "iam.jwks.url", o.JWKS.URL,
		"JWKS HTTP endpoint URL")
	fs.DurationVar(&o.JWKS.RefreshInterval, "iam.jwks.refresh-interval", o.JWKS.RefreshInterval,
		"JWKS refresh interval")
	fs.DurationVar(&o.JWKS.CacheTTL, "iam.jwks.cache-ttl", o.JWKS.CacheTTL,
		"JWKS cache TTL")

	// 缓存配置
	fs.BoolVar(&o.UserCache.Enabled, "iam.user-cache.enabled", o.UserCache.Enabled,
		"Enable user info cache")
	fs.DurationVar(&o.UserCache.TTL, "iam.user-cache.ttl", o.UserCache.TTL,
		"User cache TTL")
	fs.IntVar(&o.UserCache.MaxSize, "iam.user-cache.max-size", o.UserCache.MaxSize,
		"User cache max size")
}
