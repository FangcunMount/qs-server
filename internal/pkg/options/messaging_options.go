package options

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/messaging/nsq"
	"github.com/FangcunMount/component-base/pkg/messaging/rabbitmq"
	"github.com/spf13/pflag"
)

// MessagingOptions 消息队列配置选项
type MessagingOptions struct {
	// Enabled 是否启用消息队列
	Enabled bool `json:"enabled" mapstructure:"enabled"`

	// Provider 消息队列提供者 (nsq, rabbitmq)
	Provider string `json:"provider" mapstructure:"provider"`

	// NSQ 配置
	NSQAddr        string `json:"nsq_addr" mapstructure:"nsq-addr"`
	NSQLookupdAddr string `json:"nsq_lookupd_addr" mapstructure:"nsq-lookupd-addr"`

	// RabbitMQ 配置
	RabbitMQURL          string `json:"rabbitmq_url" mapstructure:"rabbitmq-url"`
	RabbitMQExchange     string `json:"rabbitmq_exchange" mapstructure:"rabbitmq-exchange"`
	RabbitMQExchangeType string `json:"rabbitmq_exchange_type" mapstructure:"rabbitmq-exchange-type"`
}

// NewMessagingOptions 创建默认的消息队列配置
func NewMessagingOptions() *MessagingOptions {
	return &MessagingOptions{
		Enabled:              false, // 默认不启用，避免配置错误导致服务不可用
		Provider:             "nsq",
		NSQAddr:              "127.0.0.1:4150",
		NSQLookupdAddr:       "127.0.0.1:4161",
		RabbitMQExchange:     "qs.events",
		RabbitMQExchangeType: "topic",
	}
}

// AddFlags 添加命令行参数
func (o *MessagingOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.Enabled, "messaging.enabled", o.Enabled,
		"Enable message queue for event publishing")
	fs.StringVar(&o.Provider, "messaging.provider", o.Provider,
		"Message queue provider (nsq, rabbitmq)")
	fs.StringVar(&o.NSQAddr, "messaging.nsq-addr", o.NSQAddr,
		"NSQ daemon address for publishing")
	fs.StringVar(&o.NSQLookupdAddr, "messaging.nsq-lookupd-addr", o.NSQLookupdAddr,
		"NSQ lookupd address (optional for apiserver)")
	fs.StringVar(&o.RabbitMQURL, "messaging.rabbitmq-url", o.RabbitMQURL,
		"RabbitMQ connection URL")
	fs.StringVar(&o.RabbitMQExchange, "messaging.rabbitmq-exchange", o.RabbitMQExchange,
		"RabbitMQ exchange name")
	fs.StringVar(&o.RabbitMQExchangeType, "messaging.rabbitmq-exchange-type", o.RabbitMQExchangeType,
		"RabbitMQ exchange type (direct, topic, fanout)")
}

// Validate 验证配置
func (o *MessagingOptions) Validate() []error {
	var errs []error

	if !o.Enabled {
		return nil // 未启用时不需要验证
	}

	switch o.Provider {
	case "nsq":
		if o.NSQAddr == "" {
			errs = append(errs, fmt.Errorf("nsq-addr is required when using NSQ"))
		}
	case "rabbitmq":
		if o.RabbitMQURL == "" {
			errs = append(errs, fmt.Errorf("rabbitmq-url is required when using RabbitMQ"))
		}
	default:
		errs = append(errs, fmt.Errorf("unsupported messaging provider: %s (supported: nsq, rabbitmq)", o.Provider))
	}

	return errs
}

// NewPublisher 创建消息队列发布器
func (o *MessagingOptions) NewPublisher() (messaging.Publisher, error) {
	if !o.Enabled {
		return nil, fmt.Errorf("messaging is not enabled")
	}

	switch o.Provider {
	case "nsq":
		// 创建 NSQ Publisher
		return nsq.NewPublisher(o.NSQAddr, nil)
	case "rabbitmq":
		// 创建 RabbitMQ Publisher
		return rabbitmq.NewPublisher(o.RabbitMQURL)
	default:
		return nil, fmt.Errorf("unsupported messaging provider: %s", o.Provider)
	}
}
