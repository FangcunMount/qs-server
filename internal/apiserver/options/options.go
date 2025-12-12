package options

import (
	"encoding/json"

	"github.com/FangcunMount/component-base/pkg/log"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
)

// Options 包含所有配置项
type Options struct {
	Log                     *log.Options                           `json:"log"       mapstructure:"log"`
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server"    mapstructure:"server"`
	GRPCOptions             *genericoptions.GRPCOptions            `json:"grpc"      mapstructure:"grpc"`
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure"  mapstructure:"insecure"`
	SecureServing           *genericoptions.SecureServingOptions   `json:"secure"    mapstructure:"secure"`
	MySQLOptions            *genericoptions.MySQLOptions           `json:"mysql"     mapstructure:"mysql"`
	MigrationOptions        *genericoptions.MigrationOptions       `json:"migration" mapstructure:"migration"`
	RedisDualOptions        *genericoptions.RedisDualOptions       `json:"redis"     mapstructure:"redis"`
	MongoDBOptions          *genericoptions.MongoDBOptions         `json:"mongodb"   mapstructure:"mongodb"`
	MessagingOptions        *genericoptions.MessagingOptions       `json:"messaging" mapstructure:"messaging"`
	IAMOptions              *genericoptions.IAMOptions             `json:"iam"       mapstructure:"iam"`
}

// NewOptions 创建一个 Options 对象，包含默认参数
func NewOptions() *Options {
	return &Options{
		Log:                     log.NewOptions(),
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		GRPCOptions:             genericoptions.NewGRPCOptions(),
		InsecureServing:         genericoptions.NewInsecureServingOptions(),
		SecureServing:           genericoptions.NewSecureServingOptions(),
		MySQLOptions:            genericoptions.NewMySQLOptions(),
		MigrationOptions:        genericoptions.NewMigrationOptions(),
		RedisDualOptions:        genericoptions.NewRedisDualOptions(),
		MongoDBOptions:          genericoptions.NewMongoDBOptions(),
		MessagingOptions:        genericoptions.NewMessagingOptions(),
		IAMOptions:              genericoptions.NewIAMOptions(),
	}
}

// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.Log.AddFlags(fss.FlagSet("log"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("server"))
	o.GRPCOptions.AddFlags(fss.FlagSet("grpc"))
	o.InsecureServing.AddFlags(fss.FlagSet("insecure"))
	o.SecureServing.AddFlags(fss.FlagSet("secure"))
	o.MySQLOptions.AddFlags(fss.FlagSet("mysql"))
	o.MigrationOptions.AddFlags(fss.FlagSet("migration"))
	o.RedisDualOptions.AddFlags(fss.FlagSet("redis"))
	o.MongoDBOptions.AddFlags(fss.FlagSet("mongodb"))
	o.MessagingOptions.AddFlags(fss.FlagSet("messaging"))
	o.IAMOptions.AddFlags(fss.FlagSet("iam"))

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
