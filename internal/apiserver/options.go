package apiserver

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/pflag"
	cliflag "github.com/yshujie/questionnaire-scale/pkg/flag"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Options 包含所有配置项
type Options struct {
	Log    *log.Options   `json:"log"    mapstructure:"log"`
	Server *ServerOptions `json:"server" mapstructure:"server"`
}

// ServerOptions 服务器配置选项
type ServerOptions struct {
	Mode         string `json:"mode"          mapstructure:"mode"`
	Healthz      bool   `json:"healthz"       mapstructure:"healthz"`
	Middlewares  string `json:"middlewares"   mapstructure:"middlewares"`
	MaxPingCount int    `json:"max-ping-count" mapstructure:"max-ping-count"`
}

// NewOptions 创建一个 Options 对象，包含默认参数
func NewOptions() *Options {
	return &Options{
		Log: log.NewOptions(),
		Server: &ServerOptions{
			Mode:         "release",
			Healthz:      true,
			Middlewares:  "recovery,logger,secure,nocache,cors,dump",
			MaxPingCount: 3,
		},
	}
}

// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.Log.AddFlags(fss.FlagSet("log"))
	o.addServerFlags(fss.FlagSet("server"))
	return fss
}

// addServerFlags 添加服务器相关的命令行参数
func (o *Options) addServerFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Server.Mode, "server.mode", o.Server.Mode, "Server mode: release, debug, test")
	fs.BoolVar(&o.Server.Healthz, "server.healthz", o.Server.Healthz, "Enable health check endpoint")
	fs.StringVar(&o.Server.Middlewares, "server.middlewares", o.Server.Middlewares, "Comma-separated list of middlewares")
	fs.IntVar(&o.Server.MaxPingCount, "server.max-ping-count", o.Server.MaxPingCount, "Max ping count for health check")
}

// Validate 验证命令行参数
func (o *Options) Validate() []error {
	var errs []error

	// 验证日志配置
	errs = append(errs, o.Log.Validate()...)

	// 验证服务器配置
	if o.Server.Mode != "release" && o.Server.Mode != "debug" && o.Server.Mode != "test" {
		errs = append(errs, fmt.Errorf("invalid server mode: %s", o.Server.Mode))
	}

	if o.Server.MaxPingCount <= 0 {
		errs = append(errs, fmt.Errorf("max-ping-count must be greater than 0"))
	}

	return errs
}

// Complete 完成配置选项
func (o *Options) Complete() error {
	return nil
}

// String 返回配置的字符串表示
func (o *Options) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}
