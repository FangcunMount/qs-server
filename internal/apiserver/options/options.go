package options

import (
	"encoding/json"

	genericoptions "github.com/yshujie/questionnaire-scale/internal/pkg/options"
	cliflag "github.com/yshujie/questionnaire-scale/pkg/flag"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Options 包含所有配置项
type Options struct {
	Log                     *log.Options                           `json:"log"    mapstructure:"log"`
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server" mapstructure:"server"`
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure" mapstructure:"insecure"`
	SecureServing           *genericoptions.SecureServingOptions   `json:"secure" mapstructure:"secure"`
	MySQLOptions            *genericoptions.MySQLOptions           `json:"mysql"    mapstructure:"mysql"`
}

// NewOptions 创建一个 Options 对象，包含默认参数
func NewOptions() *Options {
	return &Options{
		Log:                     log.NewOptions(),
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		InsecureServing:         genericoptions.NewInsecureServingOptions(),
		SecureServing:           genericoptions.NewSecureServingOptions(),
		MySQLOptions:            genericoptions.NewMySQLOptions(),
	}
}

// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.Log.AddFlags(fss.FlagSet("log"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("server"))
	o.InsecureServing.AddFlags(fss.FlagSet("insecure"))
	o.SecureServing.AddFlags(fss.FlagSet("secure"))
	o.MySQLOptions.AddFlags(fss.FlagSet("mysql"))

	return fss
}

// Complete 完成配置选项
func (o *Options) Complete() error {
	return o.SecureServing.Complete()
}

// String 返回配置的字符串表示
func (o *Options) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}
