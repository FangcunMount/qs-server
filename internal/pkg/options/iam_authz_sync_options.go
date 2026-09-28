package options

import (
	"fmt"
)

const (
	DefaultIAMAuthzSyncTopic = "iam.authz.version.v2"
)

// IAMAuthzSyncOptions IAM 授权版本同步订阅配置。
type IAMAuthzSyncOptions struct {
	Enabled        bool                      `json:"enabled" mapstructure:"enabled"`
	Provider       string                    `json:"provider" mapstructure:"provider"`
	NSQLookupdAddr string                    `json:"nsq_lookupd_addr" mapstructure:"nsq-lookupd-addr"`
	Topic          string                    `json:"topic" mapstructure:"topic"`
	ChannelPrefix  string                    `json:"channel_prefix" mapstructure:"channel-prefix"`
	EphemeralNSQ   bool                      `json:"ephemeral_nsq" mapstructure:"ephemeral-nsq"`
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
	if o.Provider != "nsq" {
		errs = append(errs, fmt.Errorf("unsupported iam authz-sync provider: %s (supported: nsq)", o.Provider))
	}
	if !o.Enabled {
		if o.EphemeralNSQ {
			errs = append(errs, fmt.Errorf("iam.authz-sync.ephemeral-nsq requires enabled authz sync"))
		}
		return errs
	}
	if o.Provider == "nsq" {
		if o.NSQLookupdAddr == "" {
			errs = append(errs, fmt.Errorf("iam.authz-sync.nsq-lookupd-addr is required when using NSQ"))
		}
	}

	if o.Topic == "" {
		errs = append(errs, fmt.Errorf("iam.authz-sync.topic is required"))
	}

	return errs
}
