package options

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FangcunMount/component-base/pkg/log"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
	"github.com/FangcunMount/qs-server/pkg/pubsub"
	"github.com/spf13/pflag"
)

// Options 包含所有配置项
type Options struct {
	Log                     *log.Options                           `json:"log"           mapstructure:"log"`
	GenericServerRunOptions *genericoptions.ServerRunOptions       `json:"server"        mapstructure:"server"`
	InsecureServing         *genericoptions.InsecureServingOptions `json:"insecure"      mapstructure:"insecure"`
	SecureServing           *genericoptions.SecureServingOptions   `json:"secure"        mapstructure:"secure"`

	// GRPC 客户端配置，用于连接 apiserver
	GRPCClient *GRPCClientOptions `json:"grpc_client" mapstructure:"grpc_client"`
	// 消息队列配置
	MessageQueue *MessageQueueOptions `json:"message_queue" mapstructure:"message_queue"`
	// 并发处理配置
	Concurrency *ConcurrencyOptions `json:"concurrency" mapstructure:"concurrency"`
}

// GRPCClientOptions GRPC 客户端配置
type GRPCClientOptions struct {
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
	Timeout  int    `json:"timeout"  mapstructure:"timeout"`  // 超时时间（秒）
	Insecure bool   `json:"insecure" mapstructure:"insecure"` // 是否使用不安全连接
}

// MessageQueueOptions 消息队列配置
type MessageQueueOptions struct {
	Type     string `json:"type"     mapstructure:"type"`     // 队列类型：redis, rabbitmq, kafka
	Endpoint string `json:"endpoint" mapstructure:"endpoint"` // 队列连接地址
	Topic    string `json:"topic"    mapstructure:"topic"`    // 主题/队列名称
	Group    string `json:"group"    mapstructure:"group"`    // 消费者组
	Username string `json:"username" mapstructure:"username"` // 用户名
	Password string `json:"password" mapstructure:"password"` // 密码
}

// ToPubSubConfig 将消息队列配置转换为PubSub配置
func (m *MessageQueueOptions) ToPubSubConfig() *pubsub.Config {
	config := pubsub.DefaultConfig()
	config.Addr = m.Endpoint
	config.Password = m.Password
	config.DB = 0 // 默认使用0号数据库
	config.ConsumerGroup = m.Group
	config.Consumer = "evaluation-server-consumer"
	return config
}

// ConcurrencyOptions 并发处理配置
type ConcurrencyOptions struct {
	MaxConcurrency int `json:"max_concurrency" mapstructure:"max_concurrency"` // 最大并发数
}

// NewOptions 创建一个 Options 对象，包含默认参数
func NewOptions() *Options {
	return &Options{
		Log:                     log.NewOptions(),
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		InsecureServing:         genericoptions.NewInsecureServingOptions(),
		SecureServing:           genericoptions.NewSecureServingOptions(),

		GRPCClient: &GRPCClientOptions{
			Endpoint: "localhost:9090", // apiserver 的 GRPC 端口
			Timeout:  30,
			Insecure: true,
		},
		MessageQueue: &MessageQueueOptions{
			Type:     "redis",
			Endpoint: "localhost:6379",
			Topic:    "answersheet.saved",
			Group:    "evaluation_group",
			Username: "",
			Password: "",
		},
		Concurrency: &ConcurrencyOptions{
			MaxConcurrency: 10, // 默认最大并发数
		},
	}
}

// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.Log.AddFlags(fss.FlagSet("log"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("server"))
	o.InsecureServing.AddFlags(fss.FlagSet("insecure"))
	o.SecureServing.AddFlags(fss.FlagSet("secure"))

	o.GRPCClient.AddFlags(fss.FlagSet("grpc-client"))
	o.MessageQueue.AddFlags(fss.FlagSet("message-queue"))
	o.Concurrency.AddFlags(fss.FlagSet("concurrency"))

	return fss
}

// AddFlags 添加 GRPC 客户端相关的命令行参数
func (g *GRPCClientOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&g.Endpoint, "grpc-client.endpoint", g.Endpoint,
		"The endpoint of apiserver gRPC service.")
	fs.IntVar(&g.Timeout, "grpc-client.timeout", g.Timeout,
		"The timeout for gRPC client requests in seconds.")
	fs.BoolVar(&g.Insecure, "grpc-client.insecure", g.Insecure,
		"Whether to use insecure gRPC connection.")
}

// AddFlags 添加消息队列相关的命令行参数
func (m *MessageQueueOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&m.Type, "message-queue.type", m.Type,
		"The type of message queue (redis, rabbitmq, kafka).")
	fs.StringVar(&m.Endpoint, "message-queue.endpoint", m.Endpoint,
		"The endpoint of message queue service.")
	fs.StringVar(&m.Topic, "message-queue.topic", m.Topic,
		"The topic/queue name to subscribe.")
	fs.StringVar(&m.Group, "message-queue.group", m.Group,
		"The consumer group name.")
	fs.StringVar(&m.Username, "message-queue.username", m.Username,
		"The username for message queue authentication.")
	fs.StringVar(&m.Password, "message-queue.password", m.Password,
		"The password for message queue authentication.")
}

// AddFlags 添加并发处理相关的命令行参数
func (c *ConcurrencyOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.MaxConcurrency, "concurrency.max-concurrency", c.MaxConcurrency,
		"The maximum number of concurrent goroutines for processing.")
}

// Complete 完成配置选项
func (o *Options) Complete() error {
	return o.SecureServing.Complete()
}

// Validate 验证配置选项
func (o *Options) Validate() []error {
	var errs []error

	// 验证 gRPC 客户端配置
	if o.GRPCClient.Endpoint == "" {
		errs = append(errs, fmt.Errorf("grpc-client.endpoint cannot be empty"))
	}
	if o.GRPCClient.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("grpc-client.timeout must be greater than 0"))
	}

	// 验证消息队列配置
	if o.MessageQueue.Type == "" {
		errs = append(errs, fmt.Errorf("message-queue.type cannot be empty"))
	}
	if o.MessageQueue.Endpoint == "" {
		errs = append(errs, fmt.Errorf("message-queue.endpoint cannot be empty"))
	}
	if o.MessageQueue.Topic == "" {
		errs = append(errs, fmt.Errorf("message-queue.topic cannot be empty"))
	}
	if o.MessageQueue.Group == "" {
		errs = append(errs, fmt.Errorf("message-queue.group cannot be empty"))
	}

	// 验证并发配置
	if o.Concurrency.MaxConcurrency <= 0 {
		errs = append(errs, fmt.Errorf("concurrency.max-concurrency must be greater than 0"))
	}
	if o.Concurrency.MaxConcurrency > 100 {
		errs = append(errs, fmt.Errorf("concurrency.max-concurrency cannot be greater than 100"))
	}

	// 验证 Redis 特定配置
	if strings.ToLower(o.MessageQueue.Type) == "redis" {
		if !strings.Contains(o.MessageQueue.Endpoint, ":") {
			errs = append(errs, fmt.Errorf("redis endpoint must include port (e.g., localhost:6379)"))
		}
	}

	return errs
}

// String 返回配置的字符串表示
func (o *Options) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}
