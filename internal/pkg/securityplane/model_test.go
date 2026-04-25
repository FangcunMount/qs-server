package securityplane

import "testing"

func TestTenantScopeParsesNumericOrgID(t *testing.T) {
	t.Parallel()

	scope := NewTenantScope("42", "tenant:42")
	if scope.TenantID != "42" {
		t.Fatalf("TenantID = %q, want 42", scope.TenantID)
	}
	if !scope.HasNumericOrg || scope.OrgID != 42 {
		t.Fatalf("numeric org = (%v, %d), want (true, 42)", scope.HasNumericOrg, scope.OrgID)
	}
	if scope.CasbinDomain != "tenant:42" {
		t.Fatalf("CasbinDomain = %q, want tenant:42", scope.CasbinDomain)
	}
}

func TestTenantScopeRejectsNonNumericOrZeroOrgID(t *testing.T) {
	t.Parallel()

	for _, tenantID := range []string{"tenant-alpha", "0", ""} {
		scope := NewTenantScope(tenantID, "")
		if scope.HasNumericOrg || scope.OrgID != 0 {
			t.Fatalf("tenant %q parsed as numeric org (%v, %d)", tenantID, scope.HasNumericOrg, scope.OrgID)
		}
	}
}

func TestPrincipalCopiesSlices(t *testing.T) {
	t.Parallel()

	principal := Principal{
		Kind:   PrincipalKindUser,
		Source: PrincipalSourceHTTPJWT,
		Roles:  []string{"qs:admin"},
		AMR:    []string{"pwd"},
	}

	roles := principal.RoleNames()
	roles[0] = "mutated"
	if got := principal.Roles[0]; got != "qs:admin" {
		t.Fatalf("principal role mutated to %q", got)
	}

	amr := principal.AuthenticationMethods()
	amr[0] = "mutated"
	if got := principal.AMR[0]; got != "pwd" {
		t.Fatalf("principal amr mutated to %q", got)
	}
}

func TestAuthzSnapshotViewCopiesSlices(t *testing.T) {
	t.Parallel()

	snapshot := AuthzSnapshotView{
		Roles: []string{"qs:admin"},
		Permissions: []AuthzPermissionView{
			{Resource: "qs:*", Action: ".*"},
		},
	}

	roles := snapshot.RoleNames()
	roles[0] = "mutated"
	if got := snapshot.Roles[0]; got != "qs:admin" {
		t.Fatalf("snapshot role mutated to %q", got)
	}

	perms := snapshot.PermissionViews()
	perms[0].Resource = "mutated"
	if got := snapshot.Permissions[0].Resource; got != "qs:*" {
		t.Fatalf("snapshot permission mutated to %q", got)
	}
}

func TestServiceIdentityCopiesAudiences(t *testing.T) {
	t.Parallel()

	identity := ServiceIdentity{
		ServiceID:      "collection-server",
		Source:         ServiceIdentitySourceServiceAuth,
		TargetAudience: []string{"qs-apiserver"},
	}
	audiences := identity.Audiences()
	audiences[0] = "mutated"
	if got := identity.TargetAudience[0]; got != "qs-apiserver" {
		t.Fatalf("service audience mutated to %q", got)
	}
}
