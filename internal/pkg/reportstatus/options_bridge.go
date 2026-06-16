package reportstatus

import genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"

func ConfigFromOptions(
	reportStatus *genericoptions.ReportStatusOptions,
	signaling *genericoptions.SignalingOptions,
	service string,
) Config {
	cfg := DefaultConfig(service)
	if reportStatus != nil {
		cfg.TTL = reportStatus.TTL()
	}
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
