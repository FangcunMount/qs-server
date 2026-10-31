package options

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/spf13/pflag"
)

// MessagingOptions 消息队列配置选项
type MessagingOptions struct {
	RocketMQ *RocketMQOptions `json:"rocketmq" mapstructure:"rocketmq"`
	// Enabled 是否启用消息队列
	Enabled bool `json:"enabled" mapstructure:"enabled"`

	// Provider 消息队列提供者，当前宿主只支持 RocketMQ。
	Provider string `json:"provider" mapstructure:"provider"`

	// Retained decoding fields reject stale host configuration; no NSQ client is constructed.
	NSQAddr              string   `json:"nsq_addr" mapstructure:"nsq-addr"`
	NSQLookupdAddr       string   `json:"nsq_lookupd_addr" mapstructure:"nsq-lookupd-addr"`
	NSQDHTTPEndpoints    []string `json:"nsqd_http_endpoints" mapstructure:"nsqd-http-endpoints"`
	PrimaryWorkerChannel string   `json:"primary_worker_channel" mapstructure:"primary-worker-channel"`

	Delivery *TransportDeliveryOptions `json:"delivery" mapstructure:"delivery"`
}

type TransportDeliveryOptions struct {
	Enable      bool `json:"enable" mapstructure:"enable"`
	MaxAttempts int  `json:"max_attempts" mapstructure:"max-attempts"`
}

func NewTransportDeliveryOptions() *TransportDeliveryOptions {
	return &TransportDeliveryOptions{Enable: true, MaxAttempts: retrygovernance.HardMaxDeliveryAttempts}
}

func (o *TransportDeliveryOptions) EffectiveMaxAttempts() int {
	if o == nil {
		return retrygovernance.HardMaxDeliveryAttempts
	}
	if !o.Enable {
		return 1
	}
	return o.MaxAttempts
}

func (o *TransportDeliveryOptions) Validate(path string) []error {
	if o == nil {
		return []error{fmt.Errorf("%s is required", path)}
	}
	if o.MaxAttempts < 1 || o.MaxAttempts > retrygovernance.HardMaxDeliveryAttempts {
		return []error{fmt.Errorf("%s.max_attempts must be between 1 and %d", path, retrygovernance.HardMaxDeliveryAttempts)}
	}
	return nil
}

// NewMessagingOptions 创建默认的消息队列配置
func NewMessagingOptions() *MessagingOptions {
	return &MessagingOptions{
		Enabled:              false, // 默认不启用，避免配置错误导致服务不可用
		Provider:             "rocketmq",
		PrimaryWorkerChannel: "qs-worker",
		Delivery:             NewTransportDeliveryOptions(),
	}
}

// AddFlags 添加命令行参数
func (o *MessagingOptions) AddFlags(fs *pflag.FlagSet) {
	if o.RocketMQ == nil {
		o.RocketMQ = &RocketMQOptions{}
	}
	o.RocketMQ.AddFlags(fs, "messaging.rocketmq")
	fs.BoolVar(&o.Enabled, "messaging.enabled", o.Enabled,
		"Enable message queue for event publishing")
	fs.StringVar(&o.Provider, "messaging.provider", o.Provider,
		"Message queue provider (rocketmq)")
	fs.StringVar(&o.PrimaryWorkerChannel, "messaging.primary-worker-channel", o.PrimaryWorkerChannel,
		"Durable channel used by the primary Worker consumers")
	if o.Delivery == nil {
		o.Delivery = NewTransportDeliveryOptions()
	}
	fs.BoolVar(&o.Delivery.Enable, "messaging.delivery.enable", o.Delivery.Enable,
		"Enable bounded transport retry")
	fs.IntVar(&o.Delivery.MaxAttempts, "messaging.delivery.max-attempts", o.Delivery.MaxAttempts,
		"Maximum transport delivery attempts before dead letter")
}

// Validate 验证配置
func (o *MessagingOptions) Validate() []error {
	errs := o.Delivery.Validate("messaging.delivery")
	if o.Provider != "rocketmq" && o.RocketMQ != nil && len(o.RocketMQ.Transactions) > 0 {
		errs = append(errs, fmt.Errorf("transactions require explicit RocketMQ provider"))
	}
	if o.Provider != "rocketmq" {
		return append(errs, fmt.Errorf("unsupported messaging provider: %s (supported: rocketmq); NSQ is retired", o.Provider))
	}
	if o.Enabled {
		if err := o.RocketMQ.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
