package options

import (
	"fmt"
)

const (
	DefaultIAMAuthzSyncTopic = "iam.authz.version"
)

// IAMAuthzSyncOptions IAM 授权版本同步订阅配置。
type IAMAuthzSyncOptions struct {
	Enabled        bool                      `json:"enabled" mapstructure:"enabled"`
	Provider       string                    `json:"provider" mapstructure:"provider"`
	NSQLookupdAddr string                    `json:"nsq_lookupd_addr" mapstructure:"nsq-lookupd-addr"`
	RabbitMQURL    string                    `json:"rabbitmq_url" mapstructure:"rabbitmq-url"`
	Topic          string                    `json:"topic" mapstructure:"topic"`
	ChannelPrefix  string                    `json:"channel_prefix" mapstructure:"channel-prefix"`
	Delivery       *TransportDeliveryOptions `json:"delivery" mapstructure:"delivery"`
}

// NewIAMAuthzSyncOptions 创建默认授权版本同步配置。
func NewIAMAuthzSyncOptions() *IAMAuthzSyncOptions {
	return &IAMAuthzSyncOptions{
		Enabled:        true,
		Provider:       "nsq",
		NSQLookupdAddr: "",
		Topic:          DefaultIAMAuthzSyncTopic,
		ChannelPrefix:  "qs-authz-sync",
		Delivery:       NewTransportDeliveryOptions(),
	}
}

// Validate 验证 authz-sync 配置。
func (o *IAMAuthzSyncOptions) Validate() []error {
	if o == nil {
		return nil
	}

	errs := o.Delivery.Validate("iam.authz-sync.delivery")
	if !o.Enabled {
		return errs
	}
	switch o.Provider {
	case "nsq":
		if o.NSQLookupdAddr == "" {
			errs = append(errs, fmt.Errorf("iam.authz-sync.nsq-lookupd-addr is required when using NSQ"))
		}
	case "rabbitmq":
		if o.RabbitMQURL == "" {
			errs = append(errs, fmt.Errorf("iam.authz-sync.rabbitmq-url is required when using RabbitMQ"))
		}
	default:
		errs = append(errs, fmt.Errorf("unsupported iam authz-sync provider: %s (supported: nsq, rabbitmq)", o.Provider))
	}

	if o.Topic == "" {
		errs = append(errs, fmt.Errorf("iam.authz-sync.topic is required"))
	}

	return errs
}
