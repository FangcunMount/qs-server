package options

import (
	"fmt"
	"net"

	"github.com/FangcunMount/qs-server/internal/pkg/server"
	"github.com/spf13/pflag"
)

// InsecureServingOptions 不安全的服务器配置选项
type InsecureServingOptions struct {
	BindAddress string `json:"bind_address" mapstructure:"bind-address"` // 绑定地址
	BindPort    int    `json:"bind_port"    mapstructure:"bind-port"`    // 绑定端口
}

// NewInsecureServingOptions 创建默认的不安全服务器配置选项
func NewInsecureServingOptions() *InsecureServingOptions {
	return &InsecureServingOptions{
		BindAddress: "127.0.0.1",
		BindPort:    9080,
	}
}

// Validate 验证InsecureServingOptions
func (s *InsecureServingOptions) Validate() []error {
	var errors []error

	if s.BindPort < 0 || s.BindPort > 65535 {
		errors = append(
			errors,
			fmt.Errorf(
				"--insecure.bind-port %v must be between 0 and 65535, inclusive. 0 for turning off insecure (HTTP) port",
				s.BindPort,
			),
		)
	}

	return errors
}

// AddFlags 添加命令行参数
func (s *InsecureServingOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.BindAddress, "insecure.bind-address", s.BindAddress, ""+
		"The IP address on which to serve the --insecure.bind-port "+
		"(set to 0.0.0.0 for all IPv4 interfaces and :: for all IPv6 interfaces).")

	fs.IntVar(&s.BindPort, "insecure.bind-port", s.BindPort, ""+
		"The port on which to serve unsecured, unauthenticated access. It is assumed "+
		"that firewall rules are set up such that this port is not reachable from outside of "+
		"the deployed machine and that port 443 on the iam public address is proxied to this "+
		"port. This is performed by nginx in the default setup. Set to zero to disable.")
}

// ApplyTo 应用配置到服务器
func (s *InsecureServingOptions) ApplyTo(c *server.Config) error {
	// 添加调试日志
	fmt.Printf("DEBUG: InsecureServingOptions.ApplyTo called with BindAddress=%s, BindPort=%d\n", s.BindAddress, s.BindPort)

	c.InsecureServing = &server.InsecureServingInfo{
		Address: net.JoinHostPort(s.BindAddress, fmt.Sprintf("%d", s.BindPort)),
	}

	fmt.Printf("DEBUG: InsecureServingInfo.Address set to: %s\n", c.InsecureServing.Address)
	return nil
}
