package configcontract

import "fmt"

// RateLimitProfile 描述一组入口限流参数。
type RateLimitProfile struct {
	GlobalQPS    float64
	GlobalBurst  int
	PerUserQPS   float64
	PerUserBurst int
}

// CacheTTLProfile 描述进程内/对象缓存 TTL 与合并策略。
type CacheTTLProfile struct {
	TTLSeconds   int
	JitterRatio  float64
	MaxEntries   int
	Singleflight bool
	SignalEvict  bool
}

// ConcurrencyPoolProfile 描述并发池容量与等待策略。
type ConcurrencyPoolProfile struct {
	MaxConcurrency int
	MaxWaitMs      int
}

// ValidateRateLimitProfile 校验限流 profile 在 enabled 时字段合理。
func ValidateRateLimitProfile(name string, enabled bool, p RateLimitProfile) []error {
	if !enabled {
		return nil
	}
	var errs []error
	if p.GlobalQPS <= 0 || p.GlobalBurst <= 0 {
		errs = append(errs, fmt.Errorf("%s global qps/burst must be greater than 0", name))
	}
	if p.PerUserQPS <= 0 || p.PerUserBurst <= 0 {
		errs = append(errs, fmt.Errorf("%s per-user qps/burst must be greater than 0", name))
	}
	return errs
}

// ValidateCacheTTLProfile 校验缓存 TTL profile。
func ValidateCacheTTLProfile(name string, enabled bool, p CacheTTLProfile) []error {
	if !enabled {
		return nil
	}
	var errs []error
	if p.TTLSeconds <= 0 {
		errs = append(errs, fmt.Errorf("%s ttl_seconds must be greater than 0 when enabled", name))
	}
	if p.MaxEntries <= 0 {
		errs = append(errs, fmt.Errorf("%s max_entries must be greater than 0 when enabled", name))
	}
	if p.JitterRatio < 0 || p.JitterRatio > 1 {
		errs = append(errs, fmt.Errorf("%s ttl_jitter_ratio must be between 0 and 1", name))
	}
	return errs
}

// ValidateConcurrencyPoolProfile 校验并发池 profile。
func ValidateConcurrencyPoolProfile(name string, p ConcurrencyPoolProfile) []error {
	var errs []error
	if p.MaxConcurrency <= 0 {
		errs = append(errs, fmt.Errorf("%s max_concurrency must be greater than 0", name))
	}
	if p.MaxWaitMs < 0 {
		errs = append(errs, fmt.Errorf("%s max_wait_ms cannot be negative", name))
	}
	return errs
}

// MaxOutboxPublishWorkers 按 MySQL max_open 计算 outbox publish_workers 上限。
func MaxOutboxPublishWorkers(mysqlMaxOpen int, ratio float64) int {
	if mysqlMaxOpen <= 0 {
		return 0
	}
	if ratio <= 0 {
		ratio = 0.8
	}
	return int(float64(mysqlMaxOpen) * ratio)
}
