package cachesignal

import (
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

func ConfigFromOptions(signaling *genericoptions.SignalingOptions, service string) Config {
	cfg := Config{Service: service, Signaling: DefaultSignalingOptions()}
	if signaling != nil && signaling.Redis != nil {
		redis := signaling.Redis
		cfg.Signaling.Enabled = redis.Enabled
		if redis.Prefix != "" {
			cfg.Signaling.Prefix = redis.Prefix
		}
		cfg.Signaling.Channel = redis.Channel
		if redis.BufferSize > 0 {
			cfg.Signaling.BufferSize = redis.BufferSize
		}
	}
	return cfg
}

func ConfigFromReportStatus(cfg reportstatus.Config) Config {
	return Config{
		Signaling: SignalingOptions{
			Enabled:    cfg.Signaling.Enabled,
			Prefix:     cfg.Signaling.Prefix,
			Channel:    cfg.Signaling.Channel,
			BufferSize: cfg.Signaling.BufferSize,
		},
		Service: cfg.Service,
	}
}
