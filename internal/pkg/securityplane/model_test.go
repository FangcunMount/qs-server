package securityplane

import "testing"

func TestOrgScopeCarriesTenantDomainAndOrgID(t *testing.T) {
	t.Parallel()

	scope := NewOrgScope("fangcun", 42, true, "tenant:fangcun")
	if scope.TenantDomain != "fangcun" {
		t.Fatalf("TenantDomain = %q, want fangcun", scope.TenantDomain)
	}
	if !scope.HasOrgID || scope.OrgID != 42 {
		t.Fatalf("org = (%v, %d), want (true, 42)", scope.HasOrgID, scope.OrgID)
	}
	if scope.AuthorizationDomain != "tenant:fangcun" {
		t.Fatalf("AuthorizationDomain = %q, want tenant:fangcun", scope.AuthorizationDomain)
	}
}

func TestOrgScopeWithoutOrg(t *testing.T) {
	t.Parallel()

	scope := NewOrgScope("fangcun", 0, false, "")
	if scope.HasOrgID || scope.OrgID != 0 {
		t.Fatalf("scope = %#v, want no org", scope)
	}
}
