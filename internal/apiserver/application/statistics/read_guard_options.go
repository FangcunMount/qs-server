package statistics

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

// StatisticsReadGuardOptions 控制统计读路径的并发合并与过载降级。
// overview 读路径映射到 load 守卫；默认开启 singleflight 与 stale fallback。
type StatisticsReadGuardOptions struct {
	ServiceSingleflight bool
	StaleOnTimeout      bool
	LoadTimeout         time.Duration
}

// ToLoadGuardPolicy 映射为 load守卫 策略配置。
func (o StatisticsReadGuardOptions) ToLoadGuardPolicy() loadguard.Policy {
	return loadguard.Policy{
		Singleflight: o.ServiceSingleflight,
		StaleOnError: o.StaleOnTimeout,
		LoadTimeout:  o.LoadTimeout,
	}
}

func DefaultStatisticsReadGuardOptions() StatisticsReadGuardOptions {
	return StatisticsReadGuardOptions{
		ServiceSingleflight: true,
		StaleOnTimeout:      true,
		LoadTimeout:         25 * time.Second,
	}
}
