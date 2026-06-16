package options

import "time"

// ReportStatusOptions Redis report_status 缓存配置。
type ReportStatusOptions struct {
	TTLSeconds int `json:"ttl_seconds" mapstructure:"ttl_seconds"`
}

func (o *ReportStatusOptions) TTL() time.Duration {
	if o == nil || o.TTLSeconds <= 0 {
		return 48 * time.Hour
	}
	return time.Duration(o.TTLSeconds) * time.Second
}

// SignalingOptions signaling 配置根节点。
type SignalingOptions struct {
	Redis *SignalingRedisOptions `json:"redis" mapstructure:"redis"`
}

// SignalingRedisOptions Redis Pub/Sub signaling 配置。
type SignalingRedisOptions struct {
	Enabled    bool   `json:"enabled" mapstructure:"enabled"`
	Prefix     string `json:"prefix" mapstructure:"prefix"`
	Channel    string `json:"channel" mapstructure:"channel"`
	BufferSize int    `json:"buffer_size" mapstructure:"buffer_size"`
}

func NewReportStatusOptions() *ReportStatusOptions {
	return &ReportStatusOptions{TTLSeconds: int((48 * time.Hour) / time.Second)}
}

func NewSignalingOptions() *SignalingOptions {
	return &SignalingOptions{
		Redis: &SignalingRedisOptions{
			Enabled:    false,
			Prefix:     "qs:signal",
			BufferSize: 100,
		},
	}
}
