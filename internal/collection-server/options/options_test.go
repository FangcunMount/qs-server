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

func TestSubmitCoalescingDefaultsAreBoundedAndEnabled(t *testing.T) {
	opts := NewOptions()
	if !opts.Submit.CoalescingEnabled {
		t.Fatal("submit coalescing must default to enabled")
	}
	if opts.Submit.ResolvedCoalescingWait() >= opts.Submit.ResolvedAcceptTimeout() {
		t.Fatalf(
			"coalescing wait %s must leave budget inside accept timeout %s",
			opts.Submit.ResolvedCoalescingWait(),
			opts.Submit.ResolvedAcceptTimeout(),
		)
	}
	if opts.Submit.ResolvedCoalescingPollInterval() > opts.Submit.ResolvedCoalescingWait() {
		t.Fatalf(
			"coalescing poll interval %s exceeds wait %s",
			opts.Submit.ResolvedCoalescingPollInterval(),
			opts.Submit.ResolvedCoalescingWait(),
		)
	}
}

func TestValidateAllowsSubmitCoalescingRollback(t *testing.T) {
	opts := NewOptions()
	opts.Submit.CoalescingEnabled = false
	opts.Submit.CoalescingWaitMs = 0
	opts.Submit.CoalescingPollIntervalMs = 0
	opts.Submit.CoalescingSignalTTLSeconds = 0

	for _, err := range opts.Validate() {
		if strings.Contains(err.Error(), "submit.coalescing_") {
			t.Fatalf("disabled coalescing must ignore tuning values: %v", err)
		}
	}
}

func TestValidateRejectsSubmitCoalescingWaitThatConsumesAcceptDeadline(t *testing.T) {
	opts := NewOptions()
	opts.Submit.CoalescingWaitMs = opts.Submit.AcceptTimeoutMs

	for _, err := range opts.Validate() {
		if strings.Contains(err.Error(), "submit.coalescing_wait_ms must be less than accept_timeout_ms") {
			return
		}
	}
	t.Fatalf("expected coalescing wait validation error, got %v", opts.Validate())
}
