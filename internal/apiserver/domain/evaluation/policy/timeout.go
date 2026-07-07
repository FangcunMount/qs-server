package policy

import "time"

// TimeoutPolicy 限定评估执行 duration。
type TimeoutPolicy struct {
	Limit time.Duration
}

// 默认TimeoutPolicy 返回默认 评估执行 timeout。
func DefaultTimeoutPolicy() TimeoutPolicy {
	return TimeoutPolicy{Limit: 30 * time.Second}
}
