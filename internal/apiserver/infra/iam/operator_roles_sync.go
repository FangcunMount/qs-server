package iam

import (
	"context"
	"strconv"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
)

// SyncOperatorRolesFromSnapshot 用 GetAuthorizationSnapshot 的 roles 列表覆盖 Operator 本地投影。
func SyncOperatorRolesFromSnapshot(ctx context.Context, loader *AuthzSnapshotLoader, orgID int64, op *domain.Operator) error {
	if loader == nil || op == nil {
		return nil
	}
	domainStr := loader.DomainForOrg(orgID)
	snap, err := loader.Load(ctx, domainStr, strconv.FormatInt(op.UserID(), 10))
	if err != nil {
		return err
	}
	out := make([]domain.Role, 0, len(snap.Roles))
	for _, r := range snap.Roles {
		out = append(out, domain.Role(r))
	}
	op.ReplaceRolesProjection(out)
	return nil
}
