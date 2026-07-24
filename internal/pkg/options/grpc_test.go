package options

import (
	"strings"
	"testing"
)

func TestGRPCOptionsValidateACLIdentityRequirements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*GRPCOptions)
		wantError string
	}{
		{
			name: "missing config file",
			mutate: func(opts *GRPCOptions) {
				opts.ACL.ConfigFile = ""
			},
			wantError: "grpc.acl.config-file",
		},
		{
			name: "non-deny policy",
			mutate: func(opts *GRPCOptions) {
				opts.ACL.DefaultPolicy = "allow"
			},
			wantError: "grpc.acl.default-policy",
		},
		{
			name: "mtls disabled",
			mutate: func(opts *GRPCOptions) {
				opts.MTLS.Enabled = false
			},
			wantError: "grpc.mtls.enabled",
		},
		{
			name: "client certificate optional",
			mutate: func(opts *GRPCOptions) {
				opts.MTLS.RequireClientCert = false
			},
			wantError: "grpc.mtls.require-client-cert",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := validACLGRPCOptions()
			tt.mutate(opts)
			errs := opts.Validate()
			if !containsValidationError(errs, tt.wantError) {
				t.Fatalf("Validate() errors = %v, want substring %q", errs, tt.wantError)
			}
		})
	}
}

func TestGRPCOptionsValidateAcceptsDenyACLWithRequiredMTLS(t *testing.T) {
	t.Parallel()

	if errs := validACLGRPCOptions().Validate(); len(errs) != 0 {
		t.Fatalf("Validate() errors = %v, want none", errs)
	}
}

func validACLGRPCOptions() *GRPCOptions {
	opts := NewGRPCOptions()
	opts.Insecure = false
	opts.TLSCertFile = "/tmp/server.crt"
	opts.TLSKeyFile = "/tmp/server.key"
	opts.MTLS.Enabled = true
	opts.MTLS.CAFile = "/tmp/ca.crt"
	opts.MTLS.RequireClientCert = true
	opts.ACL.Enabled = true
	opts.ACL.ConfigFile = "configs/grpc-acl.prod.yaml"
	opts.ACL.DefaultPolicy = "deny"
	return opts
}

func containsValidationError(errs []error, substring string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), substring) {
			return true
		}
	}
	return false
}
