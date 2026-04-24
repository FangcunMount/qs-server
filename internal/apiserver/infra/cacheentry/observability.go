package cacheentry

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
)

func ObservePayload(policyKey cachepolicy.CachePolicyKey, rawSize, storedSize int) {
	family := string(cachepolicy.FamilyFor(policyKey))
	policy := string(policyKey)
	if rawSize >= 0 {
		cacheobservability.ObserveCachePayloadBytes(family, policy, "raw", rawSize)
	}
	if storedSize >= 0 {
		cacheobservability.ObserveCachePayloadBytes(family, policy, "stored", storedSize)
	}
}

func ObserveInvalidate(policyKey cachepolicy.CachePolicyKey, result string) {
	cacheobservability.ObserveCacheWrite(string(cachepolicy.FamilyFor(policyKey)), string(policyKey), "invalidate", result)
}
