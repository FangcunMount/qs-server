package options

import (
	"strings"
	"testing"
)

func TestOptionsValidateLockProfileReference(t *testing.T) {
	opts := NewOptions()
	opts.RedisProfiles["sdk_cache"] = opts.Redis
	opts.RedisRuntime.Families["lock_lease"].AllowFallbackDefault = boolPtr(false)
	opts.RedisRuntime.Families["lock_lease"].RedisProfile = "lock_cache"

	errs := opts.Validate()
	for _, err := range errs {
		if strings.Contains(err.Error(), "redis_runtime.families.lock_lease.redis_profile references missing redis_profiles entry") {
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

func TestOptionsValidateDeliveryHardCap(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			opts := NewOptions()
			opts.Messaging.Delivery.Enable = enabled
			opts.Messaging.Delivery.MaxAttempts = 9
			for _, err := range opts.Validate() {
				if strings.Contains(err.Error(), "messaging.delivery.max_attempts must be between 1 and 8") {
					return
				}
			}
			t.Fatal("expected delivery hard-cap validation error")
		})
	}
}

func TestOptionsValidateHoldReplayHardCap(t *testing.T) {
	opts := NewOptions()
	opts.RetryGovernance.HoldReplay.MaxAttempts = 31
	for _, err := range opts.Validate() {
		if strings.Contains(err.Error(), "retry_governance.hold_replay.max_attempts must be between 1 and 30") {
			return
		}
	}
	t.Fatal("expected hold replay hard-cap validation error")
}

func TestOptionsValidateSecureGRPCRequiresCompleteMTLSIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		clear     func(*GRPCOptions)
		wantError string
	}{
		{name: "ca", clear: func(opts *GRPCOptions) { opts.TLSCAFile = "" }, wantError: "grpc.tls-ca-file"},
		{name: "certificate", clear: func(opts *GRPCOptions) { opts.TLSCertFile = "" }, wantError: "grpc.tls-cert-file"},
		{name: "key", clear: func(opts *GRPCOptions) { opts.TLSKeyFile = "" }, wantError: "grpc.tls-key-file"},
		{name: "server name", clear: func(opts *GRPCOptions) { opts.TLSServerName = "" }, wantError: "grpc.tls-server-name"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := NewOptions()
			opts.GRPC = secureWorkerGRPCOptions()
			tt.clear(opts.GRPC)
			if !containsWorkerValidationError(opts.Validate(), tt.wantError) {
				t.Fatalf("Validate() errors = %v, want substring %q", opts.Validate(), tt.wantError)
			}
		})
	}
}

func TestOptionsValidateAcceptsCompleteSecureGRPCIdentity(t *testing.T) {
	t.Parallel()

	opts := NewOptions()
	opts.GRPC = secureWorkerGRPCOptions()
	if errs := opts.Validate(); containsWorkerValidationError(errs, "grpc.") {
		t.Fatalf("Validate() gRPC errors = %v, want none", errs)
	}
}

func secureWorkerGRPCOptions() *GRPCOptions {
	return &GRPCOptions{
		ApiserverAddr: "qs-apiserver:9090",
		Insecure:      false,
		TLSCAFile:     "/tmp/ca.crt",
		TLSCertFile:   "/tmp/worker.crt",
		TLSKeyFile:    "/tmp/worker.key",
		TLSServerName: "qs-apiserver.svc",
	}
}

func containsWorkerValidationError(errs []error, substring string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), substring) {
			return true
		}
	}
	return false
}
