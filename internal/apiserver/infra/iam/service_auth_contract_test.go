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
		ServiceID:      "qs-apiserver",
		TargetAudience: []string{"iam-service"},
	}}
	identity := helper.ServiceIdentity()
	if identity.Source != securityplane.ServiceIdentitySourceServiceAuth {
		t.Fatalf("source = %q, want service_auth", identity.Source)
	}
	if identity.ServiceID != "qs-apiserver" {
		t.Fatalf("service id = %q, want qs-apiserver", identity.ServiceID)
	}
	if got := identity.Audiences(); len(got) != 1 || got[0] != "iam-service" {
		t.Fatalf("audiences = %#v, want [iam-service]", got)
	}
}
