package statistics

import "time"

// SystemStatisticsOptions 控制系统统计查询的并发合并与降级行为。
type SystemStatisticsOptions struct {
	// ServiceSingleflight 在应用层合并同一 org 的并发回源（B2）。
	ServiceSingleflight bool
	// DisableRealtimeFallback 快照未就绪时禁止走实时全表聚合（B4）。
	DisableRealtimeFallback bool
	// StaleOnTimeout 回源超时或失败时返回进程内最近成功结果（B4）。
	StaleOnTimeout bool
	// LoadTimeout 单次回源超时；0 表示不额外收紧（沿用上游 context）。
	LoadTimeout time.Duration
}

func DefaultSystemStatisticsOptions() SystemStatisticsOptions {
	return SystemStatisticsOptions{
		ServiceSingleflight:     true,
		DisableRealtimeFallback: true,
		StaleOnTimeout:          true,
		LoadTimeout:             25 * time.Second,
	}
}
