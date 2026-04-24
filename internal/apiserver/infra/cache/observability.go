package cache

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
)

func observePayload(policyKey cachepolicy.CachePolicyKey, rawSize, storedSize int) {
	cacheentry.ObservePayload(policyKey, rawSize, storedSize)
}

func observeInvalidate(policyKey cachepolicy.CachePolicyKey, result string) {
	cacheentry.ObserveInvalidate(policyKey, result)
}
