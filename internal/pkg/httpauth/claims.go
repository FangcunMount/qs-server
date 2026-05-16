package httpauth

import (
	"strconv"
	"strings"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
)

// tenantDomainFromClaims returns the IAM authorization domain (e.g. fangcun, platform).
func tenantDomainFromClaims(claims *pkgmiddleware.UserClaims) string {
	if claims == nil {
		return ""
	}
	return strings.TrimSpace(claims.TenantDomain)
}

// resolveOrgIDFromClaims derives QS business org_id from IAM claims.
func resolveOrgIDFromClaims(claims *pkgmiddleware.UserClaims) (uint64, bool) {
	if claims == nil {
		return 0, false
	}
	if raw := strings.TrimSpace(claims.OrgID); raw != "" {
		orgID, err := strconv.ParseUint(raw, 10, 64)
		if err == nil && orgID > 0 {
			return orgID, true
		}
	}
	return 0, false
}
