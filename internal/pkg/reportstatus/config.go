package reportstatus

import "time"

// Config report_status 与 signaling 配置。
type Config struct {
	TTL       time.Duration
	Signaling SignalingOptions
	Service   string
}

func DefaultConfig(service string) Config {
	return Config{
		TTL:       DefaultTTL,
		Signaling: DefaultSignalingOptions(),
		Service:   service,
	}
}

func (c Config) normalizedTTL() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return DefaultTTL
}
