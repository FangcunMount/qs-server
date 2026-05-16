package httpauth

import (
	"testing"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
)

func TestResolveOrgIDFromClaimsPrefersOrgClaim(t *testing.T) {
	t.Parallel()

	orgID, ok := resolveOrgIDFromClaims(&pkgmiddleware.UserClaims{
		TenantDomain: "fangcun",
		OrgID:        "42",
	})
	if !ok || orgID != 42 {
		t.Fatalf("org = (%d, %v), want (42, true)", orgID, ok)
	}
}

func TestTenantDomainFromClaimsDoesNotUseOrgID(t *testing.T) {
	t.Parallel()

	domain := tenantDomainFromClaims(&pkgmiddleware.UserClaims{OrgID: "1"})
	if domain != "" {
		t.Fatalf("tenant domain = %q, want empty", domain)
	}
}
