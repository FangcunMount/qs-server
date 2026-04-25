package securityprojection

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestPrincipalFromInputCopiesSlicesAndDefaults(t *testing.T) {
	in := PrincipalInput{
		Source:   securityplane.PrincipalSourceHTTPJWT,
		UserID:   "1001",
		TenantID: "2001",
		Roles:    []string{"operator"},
		AMR:      []string{"pwd"},
	}

	principal := PrincipalFromInput(in)
	in.Roles[0] = "mutated"
	in.AMR[0] = "mutated"

	if principal.Kind != securityplane.PrincipalKindUnknown {
		t.Fatalf("kind = %q, want unknown default", principal.Kind)
	}
	if principal.Source != securityplane.PrincipalSourceHTTPJWT {
		t.Fatalf("source = %q, want http_jwt", principal.Source)
	}
	if got := principal.RoleNames(); len(got) != 1 || got[0] != "operator" {
		t.Fatalf("roles = %#v, want [operator]", got)
	}
	if got := principal.AuthenticationMethods(); len(got) != 1 || got[0] != "pwd" {
		t.Fatalf("amr = %#v, want [pwd]", got)
	}
}

func TestTenantScopeFromTenantIDKeepsNumericAndRawTenant(t *testing.T) {
	scope := TenantScopeFromTenantID("42", "tenant:42")
	if !scope.HasNumericOrg || scope.OrgID != 42 {
		t.Fatalf("scope = %#v, want numeric org 42", scope)
	}
	if scope.TenantID != "42" || scope.CasbinDomain != "tenant:42" {
		t.Fatalf("scope = %#v, want raw tenant and domain", scope)
	}

	nonNumeric := TenantScopeFromTenantID("tenant-alpha", "")
	if nonNumeric.HasNumericOrg || nonNumeric.OrgID != 0 {
		t.Fatalf("nonNumeric scope = %#v, want no numeric org", nonNumeric)
	}
}

func TestServiceIdentityFromInputCopiesAudienceAndDefaults(t *testing.T) {
	in := ServiceIdentityInput{
		ServiceID:      "qs-apiserver",
		TargetAudience: []string{"iam-service"},
		CommonName:     "qs-apiserver.svc",
	}

	identity := ServiceIdentityFromInput(in)
	in.TargetAudience[0] = "mutated"

	if identity.Source != securityplane.ServiceIdentitySourceUnknown {
		t.Fatalf("source = %q, want unknown default", identity.Source)
	}
	if got := identity.Audiences(); len(got) != 1 || got[0] != "iam-service" {
		t.Fatalf("audiences = %#v, want [iam-service]", got)
	}
}
