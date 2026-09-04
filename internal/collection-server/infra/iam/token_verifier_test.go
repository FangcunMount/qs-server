package iam

import (
	"testing"

	authnv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/authn/v2"
	auth "github.com/FangcunMount/iam/v3/pkg/sdk/auth/verifier"
)

func TestVerifyOptionsAreRestrictedToAccessTokens(t *testing.T) {
	t.Parallel()

	assertAccessVerifyOptions(t, defaultVerifyOptions())
	merged := mergeVerifyOptions(&auth.VerifyOptions{
		ForceRemote:       true,
		AllowedTokenTypes: []authnv2.TokenType{authnv2.TokenType_TOKEN_TYPE_SERVICE},
	})
	assertAccessVerifyOptions(t, merged)
	if !merged.ForceRemote {
		t.Fatal("mergeVerifyOptions() must preserve ForceRemote")
	}
}

func TestVerifyRemotelyUsesAccessTokenProfile(t *testing.T) {
	t.Parallel()

	opts := remoteVerifyOptions()
	assertAccessVerifyOptions(t, opts)
	if !opts.ForceRemote {
		t.Fatal("VerifyRemotely() must force remote verification")
	}
}

func assertAccessVerifyOptions(t *testing.T, opts *auth.VerifyOptions) {
	t.Helper()
	if opts == nil || !opts.IncludeMetadata {
		t.Fatalf("VerifyOptions = %#v, want metadata", opts)
	}
	if len(opts.AllowedTokenTypes) != 1 || opts.AllowedTokenTypes[0] != authnv2.TokenType_TOKEN_TYPE_ACCESS {
		t.Fatalf("AllowedTokenTypes = %v, want access only", opts.AllowedTokenTypes)
	}
}
