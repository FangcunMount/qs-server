package options

import (
	"fmt"
	"net"
	"path"

	"github.com/spf13/pflag"

	"github.com/yshujie/questionnaire-scale/internal/pkg/server"
)

// SecureServingOptions 包含与HTTPS服务器启动相关的配置项
type SecureServingOptions struct {
	BindAddress string `json:"bind-address" mapstructure:"bind-address"`
	// BindPort 当Listener被设置时，BindPort会被忽略，即使设置为0，也会提供HTTPS服务
	BindPort int `json:"bind-port"    mapstructure:"bind-port"`
	// Required 设置为true意味着BindPort不能为0
	Required bool
	// ServerCert 是提供安全流量的TLS证书信息
	ServerCert GeneratableKeyCert `json:"tls"          mapstructure:"tls"`
	// AdvertiseAddress 是用于通告的IP地址
	AdvertiseAddress net.IP
}

// CertKey 包含与证书相关的配置项
type CertKey struct {
	// CertFile 是一个包含PEM编码证书的文件，可能包含完整的证书链
	CertFile string `json:"cert-file"        mapstructure:"cert-file"`
	// KeyFile 是一个包含PEM编码的私钥文件，用于指定的CertFile证书
	KeyFile string `json:"private-key-file" mapstructure:"private-key-file"`
}

// GeneratableKeyCert 包含与证书相关的配置项
type GeneratableKeyCert struct {
	// CertKey 允许设置一个显式的cert/key文件来使用
	CertKey CertKey `json:"cert-key" mapstructure:"cert-key"`

	// CertDirectory 指定一个目录，如果CertFile/KeyFile没有显式设置，则将生成的证书写入该目录
	// PairName 用于确定CertDirectory中的文件名
	// 如果CertDirectory和PairName没有设置，则生成一个内存中的证书
	CertDirectory string `json:"cert-dir"  mapstructure:"cert-dir"`
	// PairName 用于确定CertDirectory中的文件名
	// 它成为CertDirectory/PairName.crt和CertDirectory/PairName.key
	PairName string `json:"pair-name" mapstructure:"pair-name"`
}

// NewSecureServingOptions 创建一个SecureServingOptions对象，使用默认参数
func NewSecureServingOptions() *SecureServingOptions {
	return &SecureServingOptions{
		BindAddress: "0.0.0.0",
		BindPort:    8443,
		Required:    true,
		ServerCert: GeneratableKeyCert{
			PairName:      "iam",
			CertDirectory: "/var/run/iam",
		},
	}
}

// ApplyTo 应用运行选项到方法接收者并返回自身
func (s *SecureServingOptions) ApplyTo(c *server.Config) error {
	// SecureServing is required to serve https
	c.SecureServing = &server.SecureServingInfo{
		BindAddress: s.BindAddress,
		BindPort:    s.BindPort,
		CertKey: server.CertKey{
			CertFile: s.ServerCert.CertKey.CertFile,
			KeyFile:  s.ServerCert.CertKey.KeyFile,
		},
	}

	return nil
}

// Validate 验证命令行参数
func (s *SecureServingOptions) Validate() []error {
	if s == nil {
		return nil
	}

	errors := []error{}

	if s.Required && s.BindPort < 1 || s.BindPort > 65535 {
		errors = append(
			errors,
			fmt.Errorf(
				"--secure.bind-port %v must be between 1 and 65535, inclusive. It cannot be turned off with 0",
				s.BindPort,
			),
		)
	} else if s.BindPort < 0 || s.BindPort > 65535 {
		errors = append(errors, fmt.Errorf("--secure.bind-port %v must be between 0 and 65535, inclusive. 0 for turning off secure port", s.BindPort))
	}

	return errors
}

// AddFlags 添加命令行参数
func (s *SecureServingOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.BindAddress, "secure.bind-address", s.BindAddress, ""+
		"The IP address on which to listen for the --secure.bind-port port. The "+
		"associated interface(s) must be reachable by the rest of the engine, and by CLI/web "+
		"clients. If blank, all interfaces will be used (0.0.0.0 for all IPv4 interfaces and :: for all IPv6 interfaces).")
	desc := "The port on which to serve HTTPS with authentication and authorization."
	if s.Required {
		desc += " It cannot be switched off with 0."
	} else {
		desc += " If 0, don't serve HTTPS at all."
	}
	fs.IntVar(&s.BindPort, "secure.bind-port", s.BindPort, desc)

	fs.StringVar(&s.ServerCert.CertDirectory, "secure.tls.cert-dir", s.ServerCert.CertDirectory, ""+
		"The directory where the TLS certs are located. "+
		"If --secure.tls.cert-key.cert-file and --secure.tls.cert-key.private-key-file are provided, "+
		"this flag will be ignored.")

	fs.StringVar(&s.ServerCert.PairName, "secure.tls.pair-name", s.ServerCert.PairName, ""+
		"The name which will be used with --secure.tls.cert-dir to make a cert and key filenames. "+
		"It becomes <cert-dir>/<pair-name>.crt and <cert-dir>/<pair-name>.key")

	fs.StringVar(&s.ServerCert.CertKey.CertFile, "secure.tls.cert-key.cert-file", s.ServerCert.CertKey.CertFile, ""+
		"File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated "+
		"after server cert).")

	fs.StringVar(&s.ServerCert.CertKey.KeyFile, "secure.tls.cert-key.private-key-file",
		s.ServerCert.CertKey.KeyFile, ""+
			"File containing the default x509 private key matching --secure.tls.cert-key.cert-file.")
}

// Complete 填充任何未设置的字段，这些字段需要有效的数据
func (s *SecureServingOptions) Complete() error {
	if s == nil || s.BindPort == 0 {
		return nil
	}

	keyCert := &s.ServerCert.CertKey
	if len(keyCert.CertFile) != 0 || len(keyCert.KeyFile) != 0 {
		return nil
	}

	if len(s.ServerCert.CertDirectory) > 0 {
		if len(s.ServerCert.PairName) == 0 {
			return fmt.Errorf("--secure.tls.pair-name is required if --secure.tls.cert-dir is set")
		}
		keyCert.CertFile = path.Join(s.ServerCert.CertDirectory, s.ServerCert.PairName+".crt")
		keyCert.KeyFile = path.Join(s.ServerCert.CertDirectory, s.ServerCert.PairName+".key")
	}

	return nil
}

// CreateListener 创建一个net监听器，使用给定的地址，并返回它和端口
func CreateListener(addr string) (net.Listener, int, error) {
	network := "tcp"

	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to listen on %v: %w", addr, err)
	}

	// get port
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()

		return nil, 0, fmt.Errorf("invalid listen address: %q", ln.Addr().String())
	}

	return ln, tcpAddr.Port, nil
}
