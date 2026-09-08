package securityplane

import "testing"

func TestOrgScopeCarriesBusinessOrgID(t *testing.T) {
	t.Parallel()

	scope := NewOrgScope(42, true)
	if !scope.HasOrgID || scope.OrgID != 42 {
		t.Fatalf("org = (%v, %d), want (true, 42)", scope.HasOrgID, scope.OrgID)
	}
}

func TestOrgScopeWithoutOrg(t *testing.T) {
	t.Parallel()

	scope := NewOrgScope(0, false)
	if scope.HasOrgID || scope.OrgID != 0 {
		t.Fatalf("scope = %#v, want no org", scope)
	}
}
