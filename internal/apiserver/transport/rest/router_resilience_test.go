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
		provider, err := resiliencesubsystem.New(resiliencesubsystem.Options{RateLimit: cfg})
		if err != nil {
			panic(err)
		}
		deps.RateBudgets = provider
	}
	return NewRouter(deps)
}
