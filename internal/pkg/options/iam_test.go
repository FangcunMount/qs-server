package options

import (
	"errors"
	"testing"
)

func TestIAMOptionsValidateTokenProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*IAMOptions)
		wantErr error
	}{
		{
			name: "nil JWT config",
			mutate: func(opts *IAMOptions) {
				opts.JWT = nil
			},
			wantErr: ErrIAMJWTConfigRequired,
		},
		{
			name: "missing issuer",
			mutate: func(opts *IAMOptions) {
				opts.JWT.Issuer = ""
			},
			wantErr: ErrIAMJWTIssuerRequired,
		},
		{
			name: "missing audience",
			mutate: func(opts *IAMOptions) {
				opts.JWT.Audience = nil
			},
			wantErr: ErrIAMJWTAudienceRequired,
		},
		{
			name: "missing issuer with JWKS disabled",
			mutate: func(opts *IAMOptions) {
				opts.JWKSEnabled = false
				opts.JWT.Issuer = ""
			},
			wantErr: ErrIAMJWTIssuerRequired,
		},
		{
			name: "ES256 is unsupported",
			mutate: func(opts *IAMOptions) {
				opts.JWT.Algorithms = []string{"ES256"}
			},
			wantErr: ErrIAMJWTAlgorithmUnsupported,
		},
		{
			name: "mixed algorithms are unsupported",
			mutate: func(opts *IAMOptions) {
				opts.JWT.Algorithms = []string{"RS256", "ES256"}
			},
			wantErr: ErrIAMJWTAlgorithmUnsupported,
		},
		{
			name: "blank algorithm is unsupported",
			mutate: func(opts *IAMOptions) {
				opts.JWT.Algorithms = []string{""}
			},
			wantErr: ErrIAMJWTAlgorithmUnsupported,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := enabledIAMOptions()
			tt.mutate(opts)
			if errs := opts.Validate(); !containsError(errs, tt.wantErr) {
				t.Fatalf("Validate() errors = %v, want %v", errs, tt.wantErr)
			}
		})
	}
}

func TestIAMOptionsValidateAcceptsRS256Profile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		algorithms []string
	}{
		{name: "nil algorithms", algorithms: nil},
		{name: "empty algorithms", algorithms: []string{}},
		{name: "RS256", algorithms: []string{"RS256"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := enabledIAMOptions()
			opts.JWT.Algorithms = tt.algorithms
			if errs := opts.Validate(); len(errs) != 0 {
				t.Fatalf("Validate() errors = %v, want none", errs)
			}
		})
	}
}

func enabledIAMOptions() *IAMOptions {
	opts := NewIAMOptions()
	opts.Enabled = true
	opts.AuthzSync.Enabled = false
	return opts
}

func containsError(errs []error, target error) bool {
	for _, err := range errs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
