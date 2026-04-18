package cache

import "github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"

func observePayload(policyKey CachePolicyKey, rawSize, storedSize int) {
	family := string(PolicyFamily(policyKey))
	policy := string(policyKey)
	if rawSize >= 0 {
		cacheobservability.ObserveCachePayloadBytes(family, policy, "raw", rawSize)
	}
	if storedSize >= 0 {
		cacheobservability.ObserveCachePayloadBytes(family, policy, "stored", storedSize)
	}
}

func observeInvalidate(policyKey CachePolicyKey, result string) {
	cacheobservability.ObserveCacheWrite(string(PolicyFamily(policyKey)), string(policyKey), "invalidate", result)
}
