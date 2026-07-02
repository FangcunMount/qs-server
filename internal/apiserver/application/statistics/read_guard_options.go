package statistics

import "time"

// StatisticsReadGuardOptions 控制统计读路径的并发合并与过载降级。
type StatisticsReadGuardOptions struct {
	ServiceSingleflight bool
	StaleOnTimeout      bool
	LoadTimeout         time.Duration
}

func DefaultStatisticsReadGuardOptions() StatisticsReadGuardOptions {
	return StatisticsReadGuardOptions{
		ServiceSingleflight: true,
		StaleOnTimeout:      true,
		LoadTimeout:         25 * time.Second,
	}
}

func DefaultQuestionnaireStatisticsGuardOptions() StatisticsReadGuardOptions {
	return StatisticsReadGuardOptions{
		ServiceSingleflight: false,
		StaleOnTimeout:      true,
		LoadTimeout:         15 * time.Second,
	}
}
