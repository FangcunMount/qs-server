package transport

import (
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"time"
)

type SubscriberConfig struct {
	RocketMQ       *genericoptions.RocketMQOptions
	Provider       string
	HandlerTimeout time.Duration
}
