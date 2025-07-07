package grpcserver

import (
	"time"
)

// Config GRPC 服务器配置
type Config struct {
	BindAddress           string
	BindPort              int
	MaxMsgSize            int
	MaxConnectionAge      time.Duration
	MaxConnectionAgeGrace time.Duration
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	TLSCertFile           string
	TLSKeyFile            string
	EnableReflection      bool
	EnableHealthCheck     bool
	Insecure              bool // 是否使用不安全连接
}

// NewConfig 创建默认的 GRPC 服务器配置
func NewConfig() *Config {
	return &Config{
		BindAddress:           "0.0.0.0",
		BindPort:              9090,
		MaxMsgSize:            4 * 1024 * 1024,  // 4MB
		MaxConnectionAge:      2 * time.Hour,    // 连接最大存活时间
		MaxConnectionAgeGrace: 10 * time.Second, // 连接优雅终止等待时间
		ReadTimeout:           5 * time.Second,  // 读取超时时间
		WriteTimeout:          5 * time.Second,  // 写入超时时间
		EnableReflection:      true,             // 启用反射
		EnableHealthCheck:     true,             // 启用健康检查
		Insecure:              true,             // 默认使用不安全连接
	}
}

// CompletedConfig GRPC 服务器的完成配置
type CompletedConfig struct {
	*Config
}

// Complete 填充任何未设置的字段，这些字段是必需的，并且可以从其他字段派生出来
func (c *Config) Complete() CompletedConfig {
	// 设置默认值
	if c.BindAddress == "" {
		c.BindAddress = "0.0.0.0"
	}
	if c.BindPort == 0 {
		c.BindPort = 8090
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
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 5 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 5 * time.Second
	}

	return CompletedConfig{c}
}

// New 从给定的配置创建一个新的 GRPC 服务器实例
func (c CompletedConfig) New() (*Server, error) {
	return NewServer(c.Config)
}
