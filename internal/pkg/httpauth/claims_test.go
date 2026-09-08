package httpauth

import (
	"testing"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
)

func TestResolveOrgIDFromClaimsPrefersOrgClaim(t *testing.T) {
	t.Parallel()

	orgID, ok := resolveOrgIDFromClaims(&pkgmiddleware.UserClaims{

		OrgID: "42",
	})
	if !ok || orgID != 42 {
		t.Fatalf("org = (%d, %v), want (42, true)", orgID, ok)
	}
}

func TestMissingOrgClaimDoesNotInferOrganization(t *testing.T) {
	id, ok := resolveOrgIDFromClaims(&pkgmiddleware.UserClaims{UserID: "1"})
	if id != 0 || ok {
		t.Fatalf("unexpected organization: %d %v", id, ok)
	}
}
