package rest

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/gin-gonic/gin"
)

type catalogL1PeekFunc func(*gin.Context) bool

func (r *Router) concurrencyMaxWait() time.Duration {
	if r == nil || r.container == nil {
		return 0
	}
	opts := r.container.ConcurrencyOptions()
	if opts == nil || opts.MaxWaitMs <= 0 {
		return 0
	}
	return time.Duration(opts.MaxWaitMs) * time.Millisecond
}

func catalogConcurrencyHandlers(
	gate *concurrency.Gate,
	maxWait time.Duration,
	peek catalogL1PeekFunc,
	handlers ...gin.HandlerFunc,
) []gin.HandlerFunc {
	if gate == nil {
		return handlers
	}
	waitMW := gate.WaitMiddleware(maxWait, func(c *gin.Context) {
		WriteServiceUnavailable(c, 1)
	})
	mw := func(c *gin.Context) {
		if peek != nil && peek(c) {
			c.Next()
			return
		}
		waitMW(c)
	}
	return append([]gin.HandlerFunc{mw}, handlers...)
}

func (r *Router) catalogMaxWait() time.Duration {
	if r == nil || r.container == nil {
		return 0
	}
	opts := r.container.ConcurrencyOptions()
	if opts == nil {
		return 0
	}
	return opts.ResolvedCatalogMaxWait()
}

func (r *Router) catalogHandlers(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	return catalogConcurrencyHandlers(
		r.container.CatalogConcurrencyGate(),
		r.catalogMaxWait(),
		r.catalogL1Peek,
		handlers...,
	)
}

func tryQueryConcurrencyHandlers(gate *concurrency.Gate, handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	if gate == nil {
		return handlers
	}
	mw := gate.TryMiddleware(func(c *gin.Context) {
		WriteServiceUnavailable(c, 1)
	})
	return append([]gin.HandlerFunc{mw}, handlers...)
}

func waitQueryConcurrencyHandlers(gate *concurrency.Gate, maxWait time.Duration, handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	if gate == nil {
		return handlers
	}
	mw := gate.WaitMiddleware(maxWait, func(c *gin.Context) {
		WriteServiceUnavailable(c, 1)
	})
	return append([]gin.HandlerFunc{mw}, handlers...)
}

func waitSubmitConcurrencyHandlers(gate *concurrency.Gate, maxWait time.Duration, handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	if gate == nil {
		return handlers
	}
	mw := gate.WaitMiddleware(maxWait, func(c *gin.Context) {
		WriteServiceUnavailable(c, 1)
	})
	return append([]gin.HandlerFunc{mw}, handlers...)
}

func (r *Router) reportStatusHandlers(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	return tryQueryConcurrencyHandlers(r.container.QueryConcurrencyGate(), handlers...)
}

func (r *Router) queryHandlers(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	return waitQueryConcurrencyHandlers(r.container.QueryConcurrencyGate(), r.concurrencyMaxWait(), handlers...)
}

func (r *Router) submitHandlers(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	return waitSubmitConcurrencyHandlers(r.container.SubmitConcurrencyGate(), r.concurrencyMaxWait(), handlers...)
}

func (r *Router) rateLimitedCatalogHandlers(
	backend ratelimit.Backend,
	scope string,
	rateCfg *options.RateLimitOptions,
	globalQPS float64,
	globalBurst int,
	userQPS float64,
	userBurst int,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	return r.catalogHandlers(rateLimitedHandlers(
		backend, scope, rateCfg, globalQPS, globalBurst, userQPS, userBurst, handler,
	)...)
}

func (r *Router) rateLimitedReportStatusHandlers(
	backend ratelimit.Backend,
	scope string,
	rateCfg *options.RateLimitOptions,
	globalQPS float64,
	globalBurst int,
	userQPS float64,
	userBurst int,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	return r.reportStatusHandlers(rateLimitedHandlers(
		backend, scope, rateCfg, globalQPS, globalBurst, userQPS, userBurst, handler,
	)...)
}

func (r *Router) rateLimitedQueryHandlers(
	backend ratelimit.Backend,
	scope string,
	rateCfg *options.RateLimitOptions,
	globalQPS float64,
	globalBurst int,
	userQPS float64,
	userBurst int,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	return r.queryHandlers(rateLimitedHandlers(
		backend, scope, rateCfg, globalQPS, globalBurst, userQPS, userBurst, handler,
	)...)
}

func (r *Router) rateLimitedSubmitHandlers(
	backend ratelimit.Backend,
	scope string,
	rateCfg *options.RateLimitOptions,
	globalQPS float64,
	globalBurst int,
	userQPS float64,
	userBurst int,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	return r.submitHandlers(rateLimitedHandlers(
		backend, scope, rateCfg, globalQPS, globalBurst, userQPS, userBurst, handler,
	)...)
}
