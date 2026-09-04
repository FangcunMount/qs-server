package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	authnv2 "github.com/FangcunMount/iam/v3/api/grpc/iam/authn/v2"
	sdk "github.com/FangcunMount/iam/v3/pkg/sdk"
	authjwks "github.com/FangcunMount/iam/v3/pkg/sdk/auth/jwks"
	auth "github.com/FangcunMount/iam/v3/pkg/sdk/auth/verifier"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	contractIssuer   = "https://iam.fangcunmount.cn"
	contractAudience = "qs-api"
	contractKeyID    = "qs-server-contract-key"
)

type staticContractKeyFetcher struct {
	set jwk.Set
}

func (f *staticContractKeyFetcher) Fetch(context.Context) (jwk.Set, error) { return f.set, nil }
func (*staticContractKeyFetcher) Name() string                             { return "static-contract-key" }

type recordingRemoteVerifier struct {
	request *authnv2.VerifyTokenRequest
}

func (s *recordingRemoteVerifier) VerifyToken(_ context.Context, request *authnv2.VerifyTokenRequest) (*authnv2.VerifyTokenResponse, error) {
	s.request = request
	now := time.Now().UTC()
	return &authnv2.VerifyTokenResponse{
		Valid: true,
		Claims: &authnv2.TokenClaims{
			Subject:      "user:1001",
			UserId:       "1001",
			TenantId:     "fangcun",
			TenantDomain: "fangcun",
			Issuer:       contractIssuer,
			Audience:     []string{contractAudience},
			TokenType:    authnv2.TokenType_TOKEN_TYPE_ACCESS,
			IssuedAt:     timestamppb.New(now),
			ExpiresAt:    timestamppb.New(now.Add(time.Minute)),
		},
	}, nil
}

func TestIAMV320AccessTokenBoundary(t *testing.T) {
	t.Parallel()

	privateKey, manager := newContractJWKS(t)
	verifier, err := auth.NewTokenVerifier(contractTokenVerifyConfig(), manager, nil)
	if err != nil {
		t.Fatalf("NewTokenVerifier() error = %v", err)
	}
	now := time.Now().UTC()

	tests := []struct {
		name      string
		claims    map[string]interface{}
		callerOpt *auth.VerifyOptions
		wantOK    bool
	}{
		{
			name:   "access token",
			claims: contractClaims(now, "access"),
			wantOK: true,
		},
		{
			name: "legacy token without token type",
			claims: func() map[string]interface{} {
				claims := contractClaims(now, "access")
				delete(claims, "token_type")
				return claims
			}(),
			wantOK: true,
		},
		{
			name:      "service token stays rejected when caller tries to allow it",
			claims:    contractClaims(now, "service"),
			callerOpt: &auth.VerifyOptions{AllowedTokenTypes: []authnv2.TokenType{authnv2.TokenType_TOKEN_TYPE_SERVICE}},
		},
		{
			name: "wrong issuer",
			claims: func() map[string]interface{} {
				claims := contractClaims(now, "access")
				claims[jwt.IssuerKey] = "https://wrong.example.com"
				return claims
			}(),
		},
		{
			name: "wrong audience",
			claims: func() map[string]interface{} {
				claims := contractClaims(now, "access")
				claims[jwt.AudienceKey] = []string{"other-api"}
				return claims
			}(),
		},
		{
			name: "expired token",
			claims: func() map[string]interface{} {
				claims := contractClaims(now, "access")
				claims[jwt.ExpirationKey] = now.Add(-time.Minute)
				return claims
			}(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			token := signContractToken(t, privateKey, tt.claims)
			result, err := verifier.Verify(context.Background(), token, normalizeVerifyOptions(tt.callerOpt))
			if tt.wantOK {
				if err != nil {
					t.Fatalf("Verify() error = %v", err)
				}
				if result == nil || !result.Valid {
					t.Fatalf("Verify() result = %#v, want valid", result)
				}
				return
			}
			if err == nil {
				t.Fatalf("Verify() result = %#v, want rejection", result)
			}
		})
	}
}

func TestIAMV320RemoteRequestCarriesAccessTokenType(t *testing.T) {
	t.Parallel()

	privateKey, _ := newContractJWKS(t)
	remote := &recordingRemoteVerifier{}
	verifier, err := auth.NewTokenVerifier(contractTokenVerifyConfig(), nil, remote)
	if err != nil {
		t.Fatalf("NewTokenVerifier() error = %v", err)
	}
	token := signContractToken(t, privateKey, contractClaims(time.Now().UTC(), "access"))

	result, err := verifier.Verify(context.Background(), token, normalizeVerifyOptions(&auth.VerifyOptions{ForceRemote: true}))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if result == nil || !result.Valid {
		t.Fatalf("Verify() result = %#v, want valid", result)
	}
	if remote.request == nil {
		t.Fatal("remote VerifyToken request was not captured")
	}
	if len(remote.request.AcceptedTokenTypes) != 1 || remote.request.AcceptedTokenTypes[0] != authnv2.TokenType_TOKEN_TYPE_ACCESS {
		t.Fatalf("accepted_token_types = %v, want access only", remote.request.AcceptedTokenTypes)
	}
}

func contractTokenVerifyConfig() *sdk.TokenVerifyConfig {
	return &sdk.TokenVerifyConfig{
		AllowedIssuer:         contractIssuer,
		AllowedAudience:       []string{contractAudience},
		RequireExpirationTime: true,
		RequiredClaims:        []string{"sub", "exp", "user_id", "tenant_id"},
		Algorithms:            []string{"RS256"},
	}
}

func contractClaims(now time.Time, tokenType string) map[string]interface{} {
	return map[string]interface{}{
		jwt.SubjectKey:    "user:1001",
		jwt.IssuerKey:     contractIssuer,
		jwt.AudienceKey:   []string{contractAudience},
		jwt.IssuedAtKey:   now,
		jwt.ExpirationKey: now.Add(time.Minute),
		"user_id":         "1001",
		"tenant_id":       "fangcun",
		"token_type":      tokenType,
	}
}

func newContractJWKS(t *testing.T) (*rsa.PrivateKey, *authjwks.JWKSManager) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	publicKey, err := jwk.FromRaw(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("jwk.FromRaw() error = %v", err)
	}
	if err := publicKey.Set(jwk.KeyIDKey, contractKeyID); err != nil {
		t.Fatalf("set key ID: %v", err)
	}
	if err := publicKey.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		t.Fatalf("set algorithm: %v", err)
	}
	set := jwk.NewSet()
	if err := set.AddKey(publicKey); err != nil {
		t.Fatalf("AddKey() error = %v", err)
	}
	manager, err := authjwks.NewJWKSManager(
		&sdk.JWKSConfig{URL: "https://unused.invalid/.well-known/jwks.json"},
		authjwks.WithCustomChain(&staticContractKeyFetcher{set: set}),
		authjwks.WithCacheEnabled(false),
	)
	if err != nil {
		t.Fatalf("NewJWKSManager() error = %v", err)
	}
	t.Cleanup(manager.Stop)
	return privateKey, manager
}

func signContractToken(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]interface{}) string {
	t.Helper()

	token := jwt.New()
	for name, value := range claims {
		if err := token.Set(name, value); err != nil {
			t.Fatalf("set claim %s: %v", name, err)
		}
	}
	headers := jws.NewHeaders()
	if err := headers.Set(jwk.KeyIDKey, contractKeyID); err != nil {
		t.Fatalf("set protected key ID: %v", err)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, privateKey, jws.WithProtectedHeaders(headers)))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	return string(signed)
}
