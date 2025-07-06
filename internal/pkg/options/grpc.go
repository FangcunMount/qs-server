package options

import (
	"crypto/tls"
	"fmt"

	"github.com/spf13/pflag"
)

// GRPCOptions GRPC 服务器配置选项
type GRPCOptions struct {
	BindAddress string      // 绑定地址
	BindPort    int         // 绑定端口
	HealthzPort int         // 健康检查端口
	TLSConfig   *tls.Config // TLS 配置
}

// NewGRPCOptions 创建默认的 GRPC 配置选项
func NewGRPCOptions() *GRPCOptions {
	return &GRPCOptions{
		BindAddress: "0.0.0.0",
		BindPort:    8090,
		HealthzPort: 8091,
	}
}

// Validate 验证GRPCOptions
func (s *GRPCOptions) Validate() []error {
	var errors []error

	if s.BindPort < 0 || s.BindPort > 65535 {
		errors = append(
			errors,
			fmt.Errorf(
				"--insecure-port %v must be between 0 and 65535, inclusive. 0 for turning off insecure (HTTP) port",
				s.BindPort,
			),
		)
	}

	return errors
}

// AddFlags
func (s *GRPCOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.BindAddress, "grpc.bind-address", s.BindAddress, ""+
		"The IP address on which to serve the --grpc.bind-port(set to 0.0.0.0 for all IPv4 interfaces and :: for all IPv6 interfaces).")

	fs.IntVar(&s.BindPort, "grpc.bind-port", s.BindPort, ""+
		"The port on which to serve unsecured, unauthenticated grpc access. It is assumed "+
		"that firewall rules are set up such that this port is not reachable from outside of "+
		"the deployed machine and that port 443 on the iam public address is proxied to this "+
		"port. This is performed by nginx in the default setup. Set to zero to disable.")
}
