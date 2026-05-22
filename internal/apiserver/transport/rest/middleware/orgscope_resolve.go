package middleware

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/orgscope"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// APIServerOrgScopeResolver resolves org scope from operator membership.
// When the client omits org hints, defaultOrgID is used if the user is an active operator there.
func APIServerOrgScopeResolver(checker operatorapp.ActiveOperatorChecker, defaultOrgID uint64) orgscope.ResolveFunc {
	return func(ctx context.Context, userID uint64, requestedOrgID uint64) (uint64, error) {
		if checker == nil {
			return 0, errors.WithCode(code.ErrInternalServerError, "operator checker not configured")
		}
		userIDInt, err := safeconv.Uint64ToInt64(userID)
		if err != nil {
			return 0, errors.WithCode(code.ErrInvalidArgument, "user scope exceeds int64")
		}
		if requestedOrgID > 0 {
			if _, err := checker.RequireActive(ctx, int64(requestedOrgID), userIDInt); err != nil {
				return 0, err
			}
			return requestedOrgID, nil
		}
		if defaultOrgID > 0 {
			if _, err := checker.RequireActive(ctx, int64(defaultOrgID), userIDInt); err == nil {
				return defaultOrgID, nil
			}
		}
		return 0, orgscope.ErrUnresolved
	}
}
