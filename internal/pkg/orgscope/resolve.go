package orgscope

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ResolveFunc resolves QS business org_id after IAM JWT authentication.
// requestedOrgID comes from X-Org-Id header or org_id query when present.
type ResolveFunc func(ctx context.Context, userID uint64, requestedOrgID uint64) (uint64, error)

// DefaultOrgID is the single-tenant fallback used until explicit org selection is wired.
const DefaultOrgID uint64 = 1

// RequestedOrgIDHeader is the preferred HTTP header for org scope override.
const RequestedOrgIDHeader = "X-Org-Id"

// FixedResolver always returns defaultOrgID unless the client requests the same org.
func FixedResolver(defaultOrgID uint64) ResolveFunc {
	return func(_ context.Context, _ uint64, requestedOrgID uint64) (uint64, error) {
		if defaultOrgID == 0 {
			return 0, ErrUnresolved
		}
		if requestedOrgID > 0 && requestedOrgID != defaultOrgID {
			return 0, ErrMismatch
		}
		return defaultOrgID, nil
	}
}

// RequestedOrgIDFromHTTP reads optional org scope hints from header or query.
func RequestedOrgIDFromHTTP(c *gin.Context) uint64 {
	if c == nil || c.Request == nil {
		return 0
	}
	if raw := strings.TrimSpace(c.GetHeader(RequestedOrgIDHeader)); raw != "" {
		if orgID, err := strconv.ParseUint(raw, 10, 64); err == nil {
			return orgID
		}
	}
	if raw := strings.TrimSpace(c.Query("org_id")); raw != "" {
		if orgID, err := strconv.ParseUint(raw, 10, 64); err == nil {
			return orgID
		}
	}
	return 0
}

// RequestedOrgIDFromMetadata reads org_id from gRPC metadata keys org_id / x-org-id.
func RequestedOrgIDFromMetadata(md map[string]string) uint64 {
	if len(md) == 0 {
		return 0
	}
	for _, key := range []string{"org_id", "x-org-id", "x_org_id"} {
		if raw := strings.TrimSpace(md[key]); raw != "" {
			if orgID, err := strconv.ParseUint(raw, 10, 64); err == nil {
				return orgID
			}
		}
	}
	return 0
}

// ErrUnresolved indicates QS could not derive a business org scope for the user.
var ErrUnresolved = errUnresolved{}

type errUnresolved struct{}

func (errUnresolved) Error() string { return "organization scope could not be resolved" }

// ErrMismatch indicates the client requested an org outside the resolved scope.
var ErrMismatch = errMismatch{}

type errMismatch struct{}

func (errMismatch) Error() string {
	return "requested org_id does not match resolved organization scope"
}

