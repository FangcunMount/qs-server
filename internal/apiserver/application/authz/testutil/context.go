// Package testutil provides explicit action facts for application boundary tests.
package testutil

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

// WithPermission grants only the named action, without changing business scope.
func WithPermission(ctx context.Context, resource, action string) context.Context {
	return authz.WithSnapshot(ctx, &authz.Snapshot{Permissions: []authz.Permission{{Resource: resource, Action: action, Mode: authz.AuthorizationModeUnconditional}}})
}
