package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/gin-gonic/gin"
)

// ErrLimitExceeded 定义了限制超出错误
var ErrLimitExceeded = errors.New("Limit exceeded")

type LimitOptions struct {
	Component string
	Scope     string
	Resource  string
	Strategy  string
	Observer  resilienceplane.Observer
}

// Limit 如果达到限制，则丢弃（HTTP 状态 429）请求
func Limit(maxEventsPerSec float64, maxBurstSize int) gin.HandlerFunc {
	return LimitWithOptions(maxEventsPerSec, maxBurstSize, LimitOptions{
		Component: "http",
		Scope:     "global",
		Resource:  "request",
		Strategy:  "local",
	})
}

func LimitWithOptions(maxEventsPerSec float64, maxBurstSize int, opts LimitOptions) gin.HandlerFunc {
	policy := rateLimitPolicy(opts, "local", maxEventsPerSec, maxBurstSize)
	return LimitWithLimiter(ratelimit.NewLocalLimiter(policy), nil, opts)
}

// LimitWithLimiter adapts a transport-neutral rate limiter into Gin middleware.
func LimitWithLimiter(limiter ratelimit.RateLimiter, keyFn func(*gin.Context) string, opts LimitOptions) gin.HandlerFunc {
	observer := defaultLimitObserver(opts.Observer)
	return func(c *gin.Context) {
		key := ""
		if keyFn != nil {
			key = keyFn(c)
		}
		if limiter == nil {
			c.Next()
			return
		}

		decision := limiter.Decide(c.Request.Context(), key)
		observeDecision(c, observer, decision)
		if decision.Allowed {
			c.Next()
			return
		}

		_ = c.Error(ErrLimitExceeded)
		setRetryAfterHeader(c, decision)
		c.AbortWithStatus(http.StatusTooManyRequests)
	}
}

// LimitByKey 为不同 key 维护独立的限流器。
func LimitByKey(maxEventsPerSec float64, maxBurstSize int, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return LimitByKeyWithOptions(maxEventsPerSec, maxBurstSize, keyFn, LimitOptions{
		Component: "http",
		Scope:     "per_key",
		Resource:  "request",
		Strategy:  "local_key",
	})
}

func LimitByKeyWithOptions(maxEventsPerSec float64, maxBurstSize int, keyFn func(*gin.Context) string, opts LimitOptions) gin.HandlerFunc {
	policy := rateLimitPolicy(opts, "local_key", maxEventsPerSec, maxBurstSize)
	return LimitWithLimiter(ratelimit.NewKeyedLocalLimiter(policy), keyFn, opts)
}

func rateLimitPolicy(opts LimitOptions, defaultStrategy string, maxEventsPerSec float64, maxBurstSize int) ratelimit.RateLimitPolicy {
	policy := ratelimit.RateLimitPolicy{
		Component:     opts.Component,
		Scope:         opts.Scope,
		Resource:      opts.Resource,
		Strategy:      opts.Strategy,
		RatePerSecond: maxEventsPerSec,
		Burst:         maxBurstSize,
	}
	if policy.Strategy == "" {
		policy.Strategy = defaultStrategy
	}
	return policy
}

func observeDecision(c *gin.Context, observer resilienceplane.Observer, decision ratelimit.RateLimitDecision) {
	resilienceplane.Observe(c.Request.Context(), observer, resilienceplane.ProtectionRateLimit, decision.Subject, decision.Outcome)
}

func defaultLimitObserver(observer resilienceplane.Observer) resilienceplane.Observer {
	if observer != nil {
		return observer
	}
	return resilienceplane.DefaultObserver()
}

func setRetryAfterHeader(c *gin.Context, decision ratelimit.RateLimitDecision) {
	seconds := decision.RetryAfterSeconds
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
}
