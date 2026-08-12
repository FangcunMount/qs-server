package middleware

import (
	"errors"
	"net/http"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
	"github.com/gin-gonic/gin"
)

// ErrLimitExceeded 定义了限制超出错误
var ErrLimitExceeded = errors.New("limit exceeded")

type LimitOptions struct {
	Component string
	Scope     string
	Resource  string
	Strategy  string
	Observer  resilience.Observer
}

// LimitDegradedOpen preserves the distributed limiter's observable fail-open
// contract when its configured backend is unavailable.
func LimitDegradedOpen(opts LimitOptions) gin.HandlerFunc {
	policy := rateLimitPolicy(opts, "redis", 1, 1)
	return LimitWithLimiter(ratelimit.NewDistributedLimiter(nil, policy), nil, opts)
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
		ratelimit.ApplyRetryAfterHeader(c.Writer.Header(), decision)
		c.AbortWithStatus(http.StatusTooManyRequests)
	}
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

func observeDecision(c *gin.Context, observer resilience.Observer, decision ratelimit.RateLimitDecision) {
	resilience.Observe(c.Request.Context(), observer, resilience.ProtectionRateLimit, decision.Subject, decision.Outcome)
}

func defaultLimitObserver(observer resilience.Observer) resilience.Observer {
	if observer != nil {
		return observer
	}
	return resilience.DefaultObserver()
}
