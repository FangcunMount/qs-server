// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package server

import (
	"net"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Config 是用于配置 GenericAPIServer 的结构体
// 其成员按重要性排序
type Config struct {
	SecureServing   *SecureServingInfo
	InsecureServing *InsecureServingInfo
	Jwt             *JwtInfo
	Mode            string
	Middlewares     []string
	Healthz         bool
	EnableProfiling bool
	EnableMetrics   bool
}

// CertKey contains configuration items related to certificate.
// 包含与证书相关的配置项
type CertKey struct {
	// CertFile 包含 PEM 编码的证书，可能包含完整的证书链
	CertFile string
	// KeyFile 包含 PEM 编码的证书的私钥
	KeyFile string
}

// 包含 TLS 服务器的配置信息
type SecureServingInfo struct {
	// BindAddress 绑定地址
	BindAddress string
	// BindPort 绑定端口
	BindPort int
	// CertKey 包含证书和私钥的配置
	CertKey CertKey
}

// Address 将主机 IP 地址和主机端口号连接成一个地址字符串，例如：0.0.0.0:8443
func (s *SecureServingInfo) Address() string {
	return net.JoinHostPort(s.BindAddress, strconv.Itoa(s.BindPort))
}

// InsecureServingInfo 包含不安全 HTTP 服务器的配置信息
type InsecureServingInfo struct {
	Address string
}

// JwtInfo 定义了用于创建 JWT 认证中间件的 JWT 字段
type JwtInfo struct {
	// Realm 默认值为 "iam jwt"
	Realm string
	// Key 默认值为空
	Key string
	// Timeout 默认值为一小时
	Timeout time.Duration
	// MaxRefresh 默认值为零
	MaxRefresh time.Duration
}

// NewConfig 返回一个包含默认值的 Config 结构体
func NewConfig() *Config {
	return &Config{
		Healthz:         true,
		Mode:            gin.ReleaseMode,
		Middlewares:     []string{},
		EnableProfiling: true,
		EnableMetrics:   true,
		Jwt: &JwtInfo{
			Realm:      "qs jwt",
			Timeout:    1 * time.Hour,
			MaxRefresh: 1 * time.Hour,
		},
	}
}

// CompletedConfig 是 GenericAPIServer 的完成配置
type CompletedConfig struct {
	*Config
}

// Complete 填充任何未设置的字段，这些字段是必需的，并且可以从其他字段派生出来
// 如果需要 `ApplyOptions`，请先执行该操作。它正在修改接收者。
func (c *Config) Complete() CompletedConfig {
	return CompletedConfig{c}
}

// New 从给定的配置创建一个新的 GenericAPIServer 实例
func (c CompletedConfig) New() (*GenericAPIServer, error) {
	// setMode before gin.New()
	gin.SetMode(c.Mode)

	s := &GenericAPIServer{
		SecureServingInfo:   c.SecureServing,
		InsecureServingInfo: c.InsecureServing,
		healthz:             c.Healthz,
		enableMetrics:       c.EnableMetrics,
		enableProfiling:     c.EnableProfiling,
		middlewares:         c.Middlewares,
		Engine:              gin.New(),
	}

	initGenericAPIServer(s)

	return s, nil
}
