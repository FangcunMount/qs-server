package rest

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
	"github.com/gin-gonic/gin"
)

type catalogL1PeekFunc func(*gin.Context) bool

type admissionRoute int

const (
	admissionCatalog admissionRoute = iota
	admissionReportStatus
	admissionQuery
	admissionSubmit
	admissionWaitReport
)

type AdmissionPolicy struct {
	catalogGate    *concurrency.Gate
	queryGate      *concurrency.Gate
	submitGate     *concurrency.Gate
	waitReportGate *concurrency.Gate
	waitReport     *options.WaitReportOptions
	maxWait        time.Duration
	submitMaxWait  time.Duration
	catalogMaxWait time.Duration
	catalogPeek    catalogL1PeekFunc
}

func (r *Router) admissionPolicy() AdmissionPolicy {
	policy := AdmissionPolicy{
		maxWait:        r.concurrencyMaxWait(),
		catalogMaxWait: r.catalogMaxWait(),
		catalogPeek:    r.catalogL1Peek,
	}
	if r == nil || r.container == nil {
		return policy
	}
	policy.catalogGate = r.container.CatalogConcurrencyGate()
	policy.queryGate = r.container.QueryConcurrencyGate()
	policy.submitGate = r.container.SubmitConcurrencyGate()
	policy.waitReportGate = r.container.WaitReportConcurrencyGate()
	policy.waitReport = r.container.WaitReportOptions()
	policy.submitMaxWait = r.submitMaxWait()
	return policy
}

func (p AdmissionPolicy) Wrap(route admissionRoute, handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	switch route {
	case admissionCatalog:
		return catalogConcurrencyHandlers(p.catalogGate, p.catalogMaxWait, p.catalogPeek, handlers...)
	case admissionReportStatus:
		return tryQueryConcurrencyHandlers(p.queryGate, handlers...)
	case admissionQuery:
		return waitGateHandlers(p.queryGate, p.maxWait, handlers...)
	case admissionSubmit:
		return waitSubmitGateHandlers(p.submitGate, p.submitMaxWait, handlers...)
	case admissionWaitReport:
		return waitConcurrencyHandlers(p.waitReportGate, p.waitReport, handlers...)
	default:
		return handlers
	}
}

func (r *Router) submitMaxWait() time.Duration {
	if r == nil || r.container == nil {
		return 0
	}
	opts := r.container.SubmitOptions()
	if opts == nil || opts.GateWaitMs <= 0 {
		return 0
	}
	return time.Duration(opts.GateWaitMs) * time.Millisecond
}

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
	return r.admissionPolicy().Wrap(admissionCatalog, handlers...)
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

func waitGateHandlers(gate *concurrency.Gate, maxWait time.Duration, handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	if gate == nil {
		return handlers
	}
	mw := gate.WaitMiddleware(maxWait, func(c *gin.Context) {
		WriteServiceUnavailable(c, 1)
	})
	return append([]gin.HandlerFunc{mw}, handlers...)
}

func waitSubmitGateHandlers(gate *concurrency.Gate, maxWait time.Duration, handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	if gate == nil {
		return handlers
	}
	mw := gate.WaitMiddleware(maxWait, func(c *gin.Context) {
		resilience.ObserveSubmitGateReject()
		ratelimit.ApplyRetryAfterSeconds(c.Writer.Header(), 1)
		c.AbortWithStatusJSON(429, gin.H{"code": 429, "message": "submit gate busy"})
	})
	return append([]gin.HandlerFunc{mw}, handlers...)
}

func (r *Router) reportStatusHandlers(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	return r.admissionPolicy().Wrap(admissionReportStatus, handlers...)
}

func (r *Router) queryHandlers(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	return r.admissionPolicy().Wrap(admissionQuery, handlers...)
}

func (r *Router) submitHandlers(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	return r.admissionPolicy().Wrap(admissionSubmit, handlers...)
}

func (r *Router) waitReportHandlers(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	return r.admissionPolicy().Wrap(admissionWaitReport, handlers...)
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
		r.container.RateBudgetProvider(), backend, scope, rateCfg, globalQPS, globalBurst, userQPS, userBurst, handler,
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
		r.container.RateBudgetProvider(), backend, scope, rateCfg, globalQPS, globalBurst, userQPS, userBurst, handler,
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
		r.container.RateBudgetProvider(), backend, scope, rateCfg, globalQPS, globalBurst, userQPS, userBurst, handler,
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
		r.container.RateBudgetProvider(), backend, scope, rateCfg, globalQPS, globalBurst, userQPS, userBurst, handler,
	)...)
}
