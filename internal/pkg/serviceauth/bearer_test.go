package serviceauth

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

type staticTokenProvider struct {
	token string
	err   error
}

func (p staticTokenProvider) GetToken(context.Context) (string, error) {
	return p.token, p.err
}

func TestBearerRequestMetadata(t *testing.T) {
	md, err := BearerRequestMetadata(context.Background(), staticTokenProvider{token: "token-1"})
	if err != nil {
		t.Fatalf("BearerRequestMetadata returned error: %v", err)
	}
	if got := md["authorization"]; got != "Bearer token-1" {
		t.Fatalf("authorization = %q, want Bearer token-1", got)
	}
}

func TestBearerRequestMetadataReturnsTokenError(t *testing.T) {
	wantErr := errors.New("token unavailable")
	if _, err := BearerRequestMetadata(context.Background(), staticTokenProvider{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestBearerRequestMetadataRejectsNilProvider(t *testing.T) {
	if _, err := BearerRequestMetadata(context.Background(), nil); err == nil {
		t.Fatal("BearerRequestMetadata succeeded with nil provider")
	}
}

func TestServiceIdentity(t *testing.T) {
	identity := ServiceIdentity("qs-apiserver", []string{"iam-service"})
	if identity.Source != securityplane.ServiceIdentitySourceServiceAuth {
		t.Fatalf("source = %q, want service_auth", identity.Source)
	}
	if got := identity.Audiences(); len(got) != 1 || got[0] != "iam-service" {
		t.Fatalf("audiences = %#v, want [iam-service]", got)
	}
}
