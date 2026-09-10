package authz

import (
	"context"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// RequirePermission checks authoritative action facts, never role labels.
func RequirePermission(ctx context.Context, resource, action string) error {
	snapshot, ok := FromContext(ctx)
	if !ok || !snapshot.HasResourceAction(resource, action) {
		return cberrors.WithCode(code.ErrPermissionDenied, "缺少访问权限: %s/%s", resource, action)
	}
	return nil
}
