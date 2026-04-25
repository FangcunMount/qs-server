package iam

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestServiceAuthHelperContractWithoutSDKHelper(t *testing.T) {
	t.Parallel()

	helper := &ServiceAuthHelper{}
	if helper.RequireTransportSecurity() {
		t.Fatal("RequireTransportSecurity() = true, want false for current compatibility contract")
	}
	if _, err := helper.GetRequestMetadata(context.Background()); err == nil {
		t.Fatal("GetRequestMetadata() succeeded with nil SDK helper")
	}
	helper.Stop()
}

func TestServiceAuthHelperServiceIdentityProjection(t *testing.T) {
	t.Parallel()

	helper := &ServiceAuthHelper{config: &ServiceAuthConfig{
		ServiceID:      "collection-server",
		TargetAudience: []string{"iam-service", "qs-apiserver"},
	}}
	identity := helper.ServiceIdentity()
	if identity.Source != securityplane.ServiceIdentitySourceServiceAuth {
		t.Fatalf("source = %q, want service_auth", identity.Source)
	}
	if identity.ServiceID != "collection-server" {
		t.Fatalf("service id = %q, want collection-server", identity.ServiceID)
	}
	if got := identity.Audiences(); len(got) != 2 || got[0] != "iam-service" || got[1] != "qs-apiserver" {
		t.Fatalf("audiences = %#v, want [iam-service qs-apiserver]", got)
	}
}
