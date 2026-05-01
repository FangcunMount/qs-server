package operator

import (
	"context"
	"sort"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
)

type roleProjectionUpdater struct {
	repo domain.Repository
}

// NewRoleProjectionUpdater 创建一个将 IAM 授权快照角色投影回本地 operator 的应用服务。
func NewRoleProjectionUpdater(repo domain.Repository) OperatorRoleProjectionUpdater {
	return roleProjectionUpdater{repo: repo}
}

func (u roleProjectionUpdater) PersistFromSnapshot(ctx context.Context, op *OperatorResult, snap *authzapp.Snapshot) error {
	if u.repo == nil || op == nil || snap == nil {
		return nil
	}
	item, err := u.repo.FindByID(ctx, domain.ID(op.ID))
	if err != nil {
		return err
	}
	return persistOperatorRolesFromNames(ctx, u.repo, item, snap.RoleNames())
}

func (u roleProjectionUpdater) PersistFromSnapshotByUser(ctx context.Context, orgID int64, userID int64, snap *authzapp.Snapshot) error {
	if u.repo == nil || snap == nil {
		return nil
	}
	op, err := u.repo.FindByUser(ctx, orgID, userID)
	if err != nil {
		return err
	}
	return persistOperatorRolesFromNames(ctx, u.repo, op, snap.RoleNames())
}

func (u roleProjectionUpdater) SyncRoles(ctx context.Context, orgID int64, operatorID uint64) error {
	return nil
}

func persistOperatorRolesFromNames(ctx context.Context, repo domain.Repository, op *domain.Operator, roles []string) error {
	if repo == nil || op == nil {
		return nil
	}
	projected := make([]domain.Role, 0, len(roles))
	for _, role := range roles {
		projected = append(projected, domain.Role(role))
	}
	sort.Slice(projected, func(i, j int) bool {
		return string(projected[i]) < string(projected[j])
	})
	if operatorRolesEqual(op.Roles(), projected) {
		return nil
	}

	op.ReplaceRolesProjection(projected)
	return repo.Update(ctx, op)
}

func operatorRolesEqual(current, projected []domain.Role) bool {
	if len(current) != len(projected) {
		return false
	}
	for i := range current {
		if current[i] != projected[i] {
			return false
		}
	}
	return true
}
