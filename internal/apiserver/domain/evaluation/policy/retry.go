package policy

import "time"

// RetryPolicy 配置有界 retries 用于 临时评估失败。
type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

// 默认RetryPolicy 返回保守 retry 策略 用于 评估执行。
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, Backoff: 200 * time.Millisecond}
}
