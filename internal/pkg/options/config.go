package options

// InsecureServingConfig 不安全的服务器配置
type InsecureServingConfig struct {
	Addr string // 服务器地址
}

// SecureServingConfig 安全的服务器配置
type SecureServingConfig struct {
	Addr     string // 服务器地址
	CertFile string // 证书文件
	KeyFile  string // 密钥文件
}

// GRPCConfig GRPC 服务器配置
type GRPCConfig struct {
	Addr        string // 服务器地址
	HealthzAddr string // 健康检查地址
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Insecure *InsecureServingConfig // 不安全服务器配置
	Secure   *SecureServingConfig   // 安全服务器配置
	GRPC     *GRPCConfig            // GRPC 服务器配置
}
