package authz

import (
	"context"
	"os"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

const IndependentRoleModel = "independent-v1"

// RequirePermission checks authoritative action facts, never role labels.
func RequirePermission(ctx context.Context, resource, action string) error {
	snapshot, ok := FromContext(ctx)
	if !ok || !snapshot.HasResourceAction(resource, action) {
		return cberrors.WithCode(code.ErrPermissionDenied, "缺少访问权限: %s/%s", resource, action)
	}
	return nil
}

// RequireResultPermission is the compatibility-release cutover boundary. An
// explicit independent-v1 setting enables the reviewed behavior change only
// after IAM facts and consumers have been migrated in the maintenance window.
// Unknown modes fail closed. Remove the legacy branch in the final release.
func RequireResultPermission(ctx context.Context, resource, action string) error {
	switch os.Getenv("QS_AUTHZ_ROLE_MODEL") {
	case "", "legacy":
		return nil
	case IndependentRoleModel:
		return RequirePermission(ctx, resource, action)
	default:
		return cberrors.WithCode(code.ErrPermissionDenied, "授权角色模型配置无效")
	}
}
