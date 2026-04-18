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

func TestOptionsValidateMetricsConfig(t *testing.T) {
	opts := NewOptions()
	opts.Metrics.BindAddress = ""
	opts.Metrics.BindPort = 0

	errs := opts.Validate()
	joined := make([]string, 0, len(errs))
	for _, err := range errs {
		joined = append(joined, err.Error())
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "metrics.bind_address cannot be empty") {
		t.Fatalf("expected metrics.bind_address validation error, got %v", errs)
	}
	if !strings.Contains(all, "metrics.bind_port must be greater than 0") {
		t.Fatalf("expected metrics.bind_port validation error, got %v", errs)
	}
}
