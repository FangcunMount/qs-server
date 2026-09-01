package options

import (
	"strings"
	"testing"
	"time"
)

func TestPublishedModelCacheDefaultsAreBoundedAndDisabled(t *testing.T) {
	opts := NewOptions().Cache.Capabilities.Catalog.PublishedModel
	if opts == nil || opts.Enabled || opts.TTLSeconds != 180 || opts.TTLJitterRatio != 0.2 || opts.MaxEntries != 64 || !opts.Singleflight || !opts.SignalEvictEnabled {
		t.Fatalf("published-model cache defaults = %#v", opts)
	}
	if got := time.Duration(opts.TTLSeconds) * time.Second; got != 3*time.Minute {
		t.Fatalf("published-model TTL = %s", got)
	}
}

func TestPublishedModelCacheRawSettingsAndValidation(t *testing.T) {
	settings := map[string]any{
		"cache":         map[string]any{"policy_file": "cache/collection-server.dev.yaml"},
		"runtime_state": map[string]any{"report_status": map[string]any{"ttl_seconds": 172800}},
	}
	if err := NewOptions().ValidateRawSettings(settings); err != nil {
		t.Fatalf("ValidateRawSettings() error = %v", err)
	}

	opts := NewOptions()
	opts.Cache.Capabilities.Catalog.PublishedModel.Enabled = true
	opts.Cache.Capabilities.Catalog.PublishedModel.MaxEntries = 0
	if !containsCollectionValidationError(opts.Validate(), "published_model_cache.max_entries must be greater than 0") {
		t.Fatalf("Validate() errors = %v", opts.Validate())
	}
}

func TestCollectionRejectsInlineCachePolicy(t *testing.T) {
	err := NewOptions().ValidateRawSettings(map[string]any{"cache": map[string]any{
		"capabilities": map[string]any{"catalog": map[string]any{}},
	}})
	if err == nil || !strings.Contains(err.Error(), "unknown configuration field cache.capabilities") {
		t.Fatalf("ValidateRawSettings() error = %v", err)
	}
}

func TestResilienceControlDefaultsEnabled(t *testing.T) {
	opts := NewOptions()
	if opts.Resilience == nil || opts.Resilience.Control == nil || !opts.Resilience.Control.Enabled {
		t.Fatalf("resilience control defaults=%+v, want enabled", opts.Resilience)
	}
}

func TestCollectionIAMDoesNotEnableUnusedAuthzSync(t *testing.T) {
	opts := NewOptions()
	if opts.IAMOptions == nil || opts.IAMOptions.AuthzSync == nil {
		t.Fatal("IAM authz sync options are missing")
	}
	if opts.IAMOptions.AuthzSync.Enabled {
		t.Fatal("collection-server must not subscribe to unused IAM AuthZ snapshots")
	}
}

func TestValidateIncludesEnabledIAMConfiguration(t *testing.T) {
	opts := NewOptions()
	opts.IAMOptions.Enabled = true
	opts.IAMOptions.JWT.Issuer = ""

	for _, err := range opts.Validate() {
		if strings.Contains(err.Error(), "issuer") {
			return
		}
	}
	t.Fatalf("expected IAM issuer validation error, got %v", opts.Validate())
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

func TestSubmitDegradedLocalDefaultsAreConservativeAndEnabled(t *testing.T) {
	opts := NewOptions().RateLimit.ResolvedSubmitDegradedLocal()
	if !opts.Enabled || opts.GlobalQPS != 30 || opts.GlobalBurst != 45 || opts.UserQPS != 10 || opts.UserBurst != 15 {
		t.Fatalf("submit degraded local defaults = %+v", opts)
	}
}

func TestValidateRejectsInvalidEnabledSubmitDegradedLocalBudget(t *testing.T) {
	opts := NewOptions()
	opts.RateLimit.SubmitDegradedLocal.GlobalQPS = 0
	if !containsCollectionValidationError(opts.Validate(), "rate_limit.submit_degraded_local limits must be greater than 0") {
		t.Fatalf("Validate() errors = %v", opts.Validate())
	}

	opts.RateLimit.SubmitDegradedLocal.Enabled = false
	if containsCollectionValidationError(opts.Validate(), "rate_limit.submit_degraded_local") {
		t.Fatalf("disabled fallback must ignore tuning values: %v", opts.Validate())
	}
}

func TestValidateRejectsInvalidAIExplanationRequestBudget(t *testing.T) {
	opts := NewOptions()
	opts.RateLimit.AIExplanationRequestUserQPS = 0
	if !containsCollectionValidationError(opts.Validate(), "rate_limit.ai_explanation_request_user_* must be greater than 0") {
		t.Fatalf("Validate() errors = %v", opts.Validate())
	}

	opts.RateLimit.AIExplanationRequestUserQPS = 0.2
	if containsCollectionValidationError(opts.Validate(), "rate_limit.ai_explanation_request") {
		t.Fatalf("valid AI explanation request budget errors = %v", opts.Validate())
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

func TestValidateSecureGRPCClientRequiresCompleteMTLSIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		clear     func(*GRPCClientOptions)
		wantError string
	}{
		{name: "ca", clear: func(opts *GRPCClientOptions) { opts.TLSCAFile = "" }, wantError: "grpc_client.tls-ca-file"},
		{name: "certificate", clear: func(opts *GRPCClientOptions) { opts.TLSCertFile = "" }, wantError: "grpc_client.tls-cert-file"},
		{name: "key", clear: func(opts *GRPCClientOptions) { opts.TLSKeyFile = "" }, wantError: "grpc_client.tls-key-file"},
		{name: "server name", clear: func(opts *GRPCClientOptions) { opts.TLSServerName = "" }, wantError: "grpc_client.tls-server-name"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := NewOptions()
			opts.GRPCClient = secureCollectionGRPCOptions()
			tt.clear(opts.GRPCClient)
			if !containsCollectionValidationError(opts.Validate(), tt.wantError) {
				t.Fatalf("Validate() errors = %v, want substring %q", opts.Validate(), tt.wantError)
			}
		})
	}
}

func TestValidateAcceptsCompleteSecureGRPCClientIdentity(t *testing.T) {
	t.Parallel()

	opts := NewOptions()
	opts.GRPCClient = secureCollectionGRPCOptions()
	if errs := opts.Validate(); containsCollectionValidationError(errs, "grpc_client.") {
		t.Fatalf("Validate() gRPC client errors = %v, want none", errs)
	}
}

func secureCollectionGRPCOptions() *GRPCClientOptions {
	opts := NewOptions().GRPCClient
	opts.Endpoint = "qs-apiserver:9090"
	opts.Insecure = false
	opts.TLSCAFile = "/tmp/ca.crt"
	opts.TLSCertFile = "/tmp/collection.crt"
	opts.TLSKeyFile = "/tmp/collection.key"
	opts.TLSServerName = "qs-apiserver.svc"
	return opts
}

func containsCollectionValidationError(errs []error, substring string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), substring) {
			return true
		}
	}
	return false
}
