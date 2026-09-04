package configcontract

import (
	"context"
	"path/filepath"
	"testing"

	authnv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/authn/v2"
	sdk "github.com/FangcunMount/iam/v3/pkg/sdk"
	auth "github.com/FangcunMount/iam/v3/pkg/sdk/auth/verifier"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	collectionoptions "github.com/FangcunMount/qs-server/internal/collection-server/options"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

type noNetworkVerifyClient struct{}

func (noNetworkVerifyClient) VerifyToken(context.Context, *authnv2.VerifyTokenRequest) (*authnv2.VerifyTokenResponse, error) {
	return nil, nil
}

func TestVersionedIAMConfigsConstructV320TokenVerifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		load func(*testing.T) *genericoptions.IAMOptions
	}{
		{
			name: "apiserver.dev.yaml",
			load: func(t *testing.T) *genericoptions.IAMOptions {
				opts := apiserveroptions.NewOptions()
				loadConfig(t, filepath.Join(repoRoot(t), "configs", "apiserver.dev.yaml"), opts)
				return opts.IAMOptions
			},
		},
		{
			name: "apiserver.prod.yaml",
			load: func(t *testing.T) *genericoptions.IAMOptions {
				opts := apiserveroptions.NewOptions()
				loadConfig(t, filepath.Join(repoRoot(t), "configs", "apiserver.prod.yaml"), opts)
				return opts.IAMOptions
			},
		},
		{
			name: "collection-server.dev.yaml",
			load: func(t *testing.T) *genericoptions.IAMOptions {
				opts := collectionoptions.NewOptions()
				loadConfig(t, filepath.Join(repoRoot(t), "configs", "collection-server.dev.yaml"), opts)
				return opts.IAMOptions
			},
		},
		{
			name: "collection-server.prod.yaml",
			load: func(t *testing.T) *genericoptions.IAMOptions {
				opts := collectionoptions.NewOptions()
				loadConfig(t, filepath.Join(repoRoot(t), "configs", "collection-server.prod.yaml"), opts)
				return opts.IAMOptions
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			iamOpts := tt.load(t)
			if iamOpts == nil || iamOpts.JWT == nil {
				t.Fatal("IAM JWT config must be present")
			}
			verifier, err := auth.NewTokenVerifier(&sdk.TokenVerifyConfig{
				AllowedAudience:       iamOpts.JWT.Audience,
				AllowedIssuer:         iamOpts.JWT.Issuer,
				ClockSkew:             iamOpts.JWT.ClockSkew,
				RequireExpirationTime: true,
				RequiredClaims:        iamOpts.JWT.RequiredClaims,
				Algorithms:            iamOpts.JWT.Algorithms,
			}, nil, noNetworkVerifyClient{})
			if err != nil {
				t.Fatalf("NewTokenVerifier() error = %v", err)
			}
			if verifier == nil {
				t.Fatal("NewTokenVerifier() returned nil")
			}
		})
	}
}
