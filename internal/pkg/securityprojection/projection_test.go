package securityprojection

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestPrincipalFromInputCopiesSlicesAndDefaults(t *testing.T) {
	in := PrincipalInput{
		Source:       securityplane.PrincipalSourceHTTPJWT,
		UserID:       "1001",
		TenantDomain: "fangcun",
		Roles:        []string{"operator"},
		AMR:          []string{"pwd"},
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

func TestOrgScopeFromIdentity(t *testing.T) {
	scope := OrgScopeFromIdentity("fangcun", 42, true, "tenant:fangcun")
	if !scope.HasOrgID || scope.OrgID != 42 || scope.TenantDomain != "fangcun" {
		t.Fatalf("scope = %#v, want fangcun org 42", scope)
	}

	emptyOrg := OrgScopeFromIdentity("fangcun", 0, false, "")
	if emptyOrg.HasOrgID || emptyOrg.OrgID != 0 {
		t.Fatalf("scope = %#v, want no org", emptyOrg)
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
