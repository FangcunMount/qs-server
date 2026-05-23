package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/FangcunMount/component-base/pkg/errors"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/httpauth"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"github.com/gin-gonic/gin"
)

// APIServerOrgScopeResolver resolves org scope from operator membership.
// When the client omits org hints, the active operator membership is resolved from QS data.
func APIServerOrgScopeResolver(checker operatorapp.ActiveOperatorChecker) orgscope.ResolveFunc {
	return func(ctx context.Context, userID uint64, requestedOrgID uint64) (uint64, error) {
		if checker == nil {
			return 0, errors.WithCode(code.ErrInternalServerError, "operator checker not configured")
		}
		userIDInt, err := safeconv.Uint64ToInt64(userID)
		if err != nil {
			return 0, errors.WithCode(code.ErrInvalidArgument, "user scope exceeds int64")
		}
		requestedOrgIDInt, err := safeconv.Uint64ToInt64(requestedOrgID)
		if err != nil {
			return 0, errors.WithCode(code.ErrInvalidArgument, "organization scope exceeds int64")
		}
		op, err := checker.ResolveActive(ctx, userIDInt, requestedOrgIDInt)
		if err != nil {
			return 0, err
		}
		if op == nil || op.OrgID <= 0 {
			return 0, orgscope.ErrUnresolved
		}
		return safeconv.Int64ToUint64(op.OrgID)
	}
}

// ResolveOperatorOrgScopeMiddleware resolves QS business org scope from active
// operator membership and injects both OrgScope and CurrentOperator.
func ResolveOperatorOrgScopeMiddleware(checker operatorapp.ActiveOperatorChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "operator checker not configured"})
			c.Abort()
			return
		}
		claims := pkgmiddleware.GetUserClaims(c)
		if claims == nil || claims.UserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			c.Abort()
			return
		}
		userID, err := safeconv.Uint64ToInt64(GetUserID(c))
		if err != nil || userID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id format"})
			c.Abort()
			return
		}
		requestedOrgID, err := safeconv.Uint64ToInt64(orgscope.RequestedOrgIDFromHTTP(c))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "organization scope exceeds int64"})
			c.Abort()
			return
		}
		op, err := checker.ResolveActive(c.Request.Context(), userID, requestedOrgID)
		if err != nil {
			writeOperatorScopeResolveError(c, err)
			return
		}
		if op == nil || op.OrgID <= 0 {
			writeOperatorScopeResolveError(c, orgscope.ErrUnresolved)
			return
		}
		orgID, err := safeconv.Int64ToUint64(op.OrgID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "organization scope is invalid"})
			c.Abort()
			return
		}
		httpauth.ApplyResolvedOrgScope(c, claims, orgID)
		c.Set(CurrentOperatorKey, op)
		c.Next()
	}
}

func writeOperatorScopeResolveError(c *gin.Context, err error) {
	status := orgscope.HTTPStatusForResolveError(err)
	if errors.IsCode(err, code.ErrInvalidArgument) {
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"error": fmt.Sprintf("organization scope could not be resolved: %v", err)})
	c.Abort()
}
