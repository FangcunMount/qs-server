package rest

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/apiserver/resilience/subsystem"
)

func newRouterWithBudgets(deps Deps) *Router {
	cfg := deps.RateLimit
	if cfg == nil {
		cfg = options.NewRateLimitOptions()
		deps.RateLimit = cfg
	}
	if deps.RateBudgets == nil {
		deps.RateBudgets = resiliencesubsystem.New(resiliencesubsystem.Options{RateLimit: cfg})
	}
	return NewRouter(deps)
}
