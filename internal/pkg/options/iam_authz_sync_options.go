package options

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/component-base/pkg/messaging/nsq"
	"github.com/FangcunMount/component-base/pkg/messaging/rabbitmq"
)

const (
	DefaultIAMAuthzSyncTopic = "iam.authz.version"
)

// IAMAuthzSyncOptions IAM 授权版本同步订阅配置。
type IAMAuthzSyncOptions struct {
	Enabled        bool   `json:"enabled" mapstructure:"enabled"`
	Provider       string `json:"provider" mapstructure:"provider"`
	NSQLookupdAddr string `json:"nsq_lookupd_addr" mapstructure:"nsq-lookupd-addr"`
	RabbitMQURL    string `json:"rabbitmq_url" mapstructure:"rabbitmq-url"`
	Topic          string `json:"topic" mapstructure:"topic"`
	ChannelPrefix  string `json:"channel_prefix" mapstructure:"channel-prefix"`
}

// NewIAMAuthzSyncOptions 创建默认授权版本同步配置。
func NewIAMAuthzSyncOptions() *IAMAuthzSyncOptions {
	return &IAMAuthzSyncOptions{
		Enabled:        true,
		Provider:       "nsq",
		NSQLookupdAddr: "",
		Topic:          DefaultIAMAuthzSyncTopic,
		ChannelPrefix:  "qs-authz-sync",
	}
}

// Validate 验证 authz-sync 配置。
func (o *IAMAuthzSyncOptions) Validate() []error {
	if o == nil || !o.Enabled {
		return nil
	}

	var errs []error
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

// NewSubscriber 创建授权版本同步订阅者。
func (o *IAMAuthzSyncOptions) NewSubscriber() (messaging.Subscriber, error) {
	if o == nil || !o.Enabled {
		return nil, fmt.Errorf("iam authz-sync is not enabled")
	}

	switch o.Provider {
	case "nsq":
		return nsq.NewSubscriber([]string{o.NSQLookupdAddr}, nil)
	case "rabbitmq":
		return rabbitmq.NewSubscriber(o.RabbitMQURL)
	default:
		return nil, fmt.Errorf("unsupported iam authz-sync provider: %s", o.Provider)
	}
}
