package options

import (
	"strings"
	"testing"
)

func TestResilienceControlDefaultsEnabled(t *testing.T) {
	opts := NewOptions()
	if opts.Resilience == nil || opts.Resilience.Control == nil || !opts.Resilience.Control.Enabled {
		t.Fatalf("resilience control defaults=%+v, want enabled", opts.Resilience)
	}
}

func TestValidateRejectsIAMTransportHardCapWhileDisabled(t *testing.T) {
	opts := NewOptions()
	opts.IAMOptions.AuthzSync.Delivery.Enable = false
	opts.IAMOptions.AuthzSync.Delivery.MaxAttempts = 9
	for _, err := range opts.Validate() {
		if strings.Contains(err.Error(), "iam.authz-sync.delivery.max_attempts must be between 1 and 8") {
			return
		}
	}
	t.Fatalf("expected IAM transport hard-cap error, got %v", opts.Validate())
}

func TestValidateAllowsMissingProfileWhenRuntimeFamilyFallsBackToDefault(t *testing.T) {
	opts := NewOptions()
	opts.RedisRuntime.Families["ops_runtime"].RedisProfile = "missing_profile"
	opts.RedisRuntime.Families["ops_runtime"].AllowFallbackDefault = boolPtr(true)

	for _, err := range opts.Validate() {
		if strings.Contains(err.Error(), "redis_runtime.families.ops_runtime.redis_profile references missing redis_profiles entry") {
			t.Fatalf("unexpected runtime validation error: %v", err)
		}
	}
}

func TestValidateRejectsMissingProfileWhenFallbackDisabled(t *testing.T) {
	opts := NewOptions()
	opts.RedisRuntime.Families["lock_lease"].RedisProfile = "missing_profile"
	opts.RedisRuntime.Families["lock_lease"].AllowFallbackDefault = boolPtr(false)

	var found bool
	for _, err := range opts.Validate() {
		if strings.Contains(err.Error(), "redis_runtime.families.lock_lease.redis_profile references missing redis_profiles entry") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing redis_runtime profile validation error, got: %v", opts.Validate())
	}
}
