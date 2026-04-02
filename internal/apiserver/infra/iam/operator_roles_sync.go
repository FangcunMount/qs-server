package iam

import (
	"context"
	"sort"
	"strconv"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
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
	ReplaceOperatorRolesProjectionFromSnapshot(op, snap)
	return nil
}

// ReplaceOperatorRolesProjectionFromSnapshot 用授权快照角色替换本地投影；返回是否发生变更。
func ReplaceOperatorRolesProjectionFromSnapshot(op *domain.Operator, snap *authzapp.Snapshot) bool {
	if op == nil || snap == nil {
		return false
	}
	out := make([]domain.Role, 0, len(snap.Roles))
	for _, r := range snap.Roles {
		out = append(out, domain.Role(r))
	}
	sort.Slice(out, func(i, j int) bool {
		return string(out[i]) < string(out[j])
	})
	if operatorRolesEqual(op.Roles(), out) {
		return false
	}
	op.ReplaceRolesProjection(out)
	return true
}

// PersistOperatorRolesProjectionFromSnapshot 在快照角色发生变化时持久化本地投影。
func PersistOperatorRolesProjectionFromSnapshot(
	ctx context.Context,
	repo domain.Repository,
	op *domain.Operator,
	snap *authzapp.Snapshot,
) (bool, error) {
	if repo == nil || op == nil || snap == nil {
		return false, nil
	}
	changed := ReplaceOperatorRolesProjectionFromSnapshot(op, snap)
	if !changed {
		return false, nil
	}
	return true, repo.Update(ctx, op)
}

// SyncAndPersistOperatorRolesFromSnapshot 从 IAM 拉取快照并在必要时持久化角色投影。
func SyncAndPersistOperatorRolesFromSnapshot(
	ctx context.Context,
	loader *AuthzSnapshotLoader,
	repo domain.Repository,
	orgID int64,
	op *domain.Operator,
) (bool, error) {
	if loader == nil || repo == nil || op == nil {
		return false, nil
	}
	domainStr := loader.DomainForOrg(orgID)
	snap, err := loader.Load(ctx, domainStr, strconv.FormatInt(op.UserID(), 10))
	if err != nil {
		return false, err
	}
	return PersistOperatorRolesProjectionFromSnapshot(ctx, repo, op, snap)
}

func operatorRolesEqual(a, b []domain.Role) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
