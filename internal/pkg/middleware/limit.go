package middleware

import (
	"errors"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ErrLimitExceeded 定义了限制超出错误
var ErrLimitExceeded = errors.New("Limit exceeded")

// Limit 如果达到限制，则丢弃（HTTP 状态 429）请求
func Limit(maxEventsPerSec float64, maxBurstSize int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(maxEventsPerSec), maxBurstSize)

	return func(c *gin.Context) {
		if limiter.Allow() {
			c.Next()

			return
		}

		// Limit reached
		_ = c.Error(ErrLimitExceeded)
		setRetryAfterHeader(c, limiter)
		c.AbortWithStatus(429)
	}
}

type keyedLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// LimitByKey 为不同 key 维护独立的限流器。
func LimitByKey(maxEventsPerSec float64, maxBurstSize int, keyFn func(*gin.Context) string) gin.HandlerFunc {
	const (
		limiterTTL      = 10 * time.Minute
		cleanupInterval = 1 * time.Minute
		maxEntries      = 10000
	)

	var (
		mu          sync.Mutex
		entries     = make(map[string]*keyedLimiter)
		lastCleanup time.Time
	)

	return func(c *gin.Context) {
		key := ""
		if keyFn != nil {
			key = keyFn(c)
		}
		if key == "" {
			key = "anonymous"
		}

		now := time.Now()
		mu.Lock()
		if now.Sub(lastCleanup) >= cleanupInterval {
			for k, v := range entries {
				if now.Sub(v.lastSeen) > limiterTTL {
					delete(entries, k)
				}
			}
			if len(entries) > maxEntries {
				for k := range entries {
					delete(entries, k)
					if len(entries) <= maxEntries {
						break
					}
				}
			}
			lastCleanup = now
		}

		entry := entries[key]
		if entry == nil {
			entry = &keyedLimiter{
				limiter: rate.NewLimiter(rate.Limit(maxEventsPerSec), maxBurstSize),
			}
			entries[key] = entry
		}
		entry.lastSeen = now
		allow := entry.limiter.Allow()
		mu.Unlock()

		if allow {
			c.Next()
			return
		}

		_ = c.Error(ErrLimitExceeded)
		setRetryAfterHeader(c, entry.limiter)
		c.AbortWithStatus(429)
	}
}

func setRetryAfterHeader(c *gin.Context, limiter *rate.Limiter) {
	if limiter == nil {
		c.Header("Retry-After", "1")
		return
	}

	reservation := limiter.Reserve()
	if !reservation.OK() {
		c.Header("Retry-After", "1")
		return
	}
	delay := reservation.Delay()
	reservation.CancelAt(time.Now())
	seconds := int(math.Ceil(delay.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
}
