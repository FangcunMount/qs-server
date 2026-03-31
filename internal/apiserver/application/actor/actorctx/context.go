package actorctx

import (
	"context"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type grantingUIDKey struct{}

// WithGrantingUserID 注入当前操作者（B 端）的 IAM user_id，用于 IAM GrantAssignment 的 granted_by。
func WithGrantingUserID(ctx context.Context, iamUserID uint64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, grantingUIDKey{}, iamUserID)
}

// GrantingUserID 返回当前请求的操作者 user_id；未注入时为 0。
func GrantingUserID(ctx context.Context) uint64 {
	if ctx == nil {
		return 0
	}
	v, _ := ctx.Value(grantingUIDKey{}).(uint64)
	return v
}

// IAMGrantedBySubject 返回 user:<id>，供 GrantAssignment.granted_by；无注入时返回空字符串。
func IAMGrantedBySubject(ctx context.Context) string {
	uid := GrantingUserID(ctx)
	if uid == 0 {
		return ""
	}
	return authz.SubjectKey(strconv.FormatUint(uid, 10))
}
