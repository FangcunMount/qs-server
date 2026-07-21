package delegatedsubject

import (
	"context"
	"strconv"
	"time"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
)

// SignInputFromContext builds delegation input from an authenticated HTTP request context.
func SignInputFromContext(ctx context.Context, testeeID uint64, purpose string, ttl time.Duration) (SignInput, error) {
	claims, _ := ctx.Value(pkgmiddleware.UserClaimsContextKey{}).(*pkgmiddleware.UserClaims)
	if claims == nil || claims.UserID == "" {
		return SignInput{}, ErrMissingToken
	}
	var orgID uint64
	if claims.OrgID != "" {
		parsed, err := strconv.ParseUint(claims.OrgID, 10, 64)
		if err != nil {
			return SignInput{}, err
		}
		orgID = parsed
	}
	return SignInput{
		UserID:   claims.UserID,
		TesteeID: testeeID,
		OrgID:    orgID,
		Purpose:  purpose,
		TTL:      ttl,
	}, nil
}
