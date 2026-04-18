package options

import (
	"strings"
	"testing"
)

func TestOptionsValidateLockProfileReference(t *testing.T) {
	opts := NewOptions()
	opts.RedisProfiles["sdk_cache"] = opts.Redis
	opts.Cache.Lock.RedisProfile = "lock_cache"

	errs := opts.Validate()
	for _, err := range errs {
		if strings.Contains(err.Error(), "cache.lock.redis_profile references missing redis_profiles entry") {
			return
		}
	}
	t.Fatalf("expected lock profile validation error, got %v", errs)
}
