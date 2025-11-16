package options

import (
	"fmt"
	"os"

	"github.com/FangcunMount/qs-server/internal/pkg/server"
	"github.com/spf13/pflag"
)

// SecureServingOptions 安全的服务器配置选项
type SecureServingOptions struct {
	BindAddress string `json:"bind_address" mapstructure:"bind-address"` // 绑定地址
	BindPort    int    `json:"bind_port"    mapstructure:"bind-port"`    // 绑定端口
	TLS         struct {
		CertFile string `json:"cert_file"    mapstructure:"cert-file"`        // 证书文件
		KeyFile  string `json:"key_file"     mapstructure:"private-key-file"` // 密钥文件
	} `json:"tls" mapstructure:"tls"`
}

// NewSecureServingOptions 创建默认的安全服务器配置选项
func NewSecureServingOptions() *SecureServingOptions {
	return &SecureServingOptions{
		BindAddress: "127.0.0.1",
		BindPort:    9444,
	}
}

// Validate 验证SecureServingOptions
func (s *SecureServingOptions) Validate() []error {
	var errors []error

	if s.BindPort < 0 || s.BindPort > 65535 {
		errors = append(
			errors,
			fmt.Errorf(
				"--secure.bind-port %v must be between 0 and 65535, inclusive. 0 for turning off secure (HTTPS) port",
				s.BindPort,
			),
		)
	}

	return errors
}

// Complete 完成配置选项
func (s *SecureServingOptions) Complete() error {
	fmt.Printf("SecureServingOptions: %+v\n", s)
	if s.BindPort == 0 {
		return nil
	}

	if len(s.TLS.CertFile) == 0 || len(s.TLS.KeyFile) == 0 {
		return fmt.Errorf("--secure.tls.cert-file and --secure.tls.private-key-file are required for serving via HTTPS")
	}

	// 检查证书文件是否存在
	if _, err := os.Stat(s.TLS.CertFile); err != nil {
		return fmt.Errorf("could not stat certificate file %s: %v", s.TLS.CertFile, err)
	}
	if _, err := os.Stat(s.TLS.KeyFile); err != nil {
		return fmt.Errorf("could not stat private key file %s: %v", s.TLS.KeyFile, err)
	}

	return nil
}

// AddFlags 添加命令行参数
func (s *SecureServingOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.BindAddress, "secure.bind-address", s.BindAddress, ""+
		"The IP address on which to serve the --secure.bind-port "+
		"(set to 0.0.0.0 for all IPv4 interfaces and :: for all IPv6 interfaces).")

	fs.IntVar(&s.BindPort, "secure.bind-port", s.BindPort, ""+
		"The port on which to serve secured, authenticated access. It is assumed "+
		"that firewall rules are set up such that this port is not reachable from outside of "+
		"the deployed machine and that port 443 on the iam public address is proxied to this "+
		"port. This is performed by nginx in the default setup. Set to zero to disable.")

	fs.StringVar(&s.TLS.CertFile, "secure.tls.cert-file", s.TLS.CertFile, ""+
		"File containing the default x509 Certificate for HTTPS.")

	fs.StringVar(&s.TLS.KeyFile, "secure.tls.private-key-file", s.TLS.KeyFile, ""+
		"File containing the default x509 private key matching --secure.tls.cert-file.")
}

// ApplyTo 应用配置到服务器
func (s *SecureServingOptions) ApplyTo(c *server.Config) error {
	c.SecureServing = &server.SecureServingInfo{
		BindAddress: s.BindAddress,
		BindPort:    s.BindPort,
		CertKey: server.CertKey{
			CertFile: s.TLS.CertFile,
			KeyFile:  s.TLS.KeyFile,
		},
	}
	return nil
}
