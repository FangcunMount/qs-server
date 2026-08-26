package operator

import (
	"context"
	"sort"
	"time"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
)

type roleProjectionUpdater struct {
	repo  domain.Repository
	authz iambridge.OperatorAuthzGateway
}

// NewRoleProjectionUpdater 创建一个将 IAM 授权快照角色投影回本地 operator 的应用服务。
func NewRoleProjectionUpdater(repo domain.Repository, authz iambridge.OperatorAuthzGateway) OperatorRoleProjectionUpdater {
	return roleProjectionUpdater{repo: repo, authz: authz}
}

func (u roleProjectionUpdater) PersistFromSnapshot(ctx context.Context, op *OperatorResult, snap *authzapp.Snapshot) error {
	if u.repo == nil || op == nil || snap == nil {
		return nil
	}
	item, err := u.repo.FindByID(ctx, domain.ID(op.ID))
	if err != nil {
		return err
	}
	return persistOperatorRolesFromSnapshot(ctx, u.repo, item, snap)
}

func (u roleProjectionUpdater) PersistFromSnapshotByUser(ctx context.Context, orgID int64, userID int64, snap *authzapp.Snapshot) error {
	if u.repo == nil || snap == nil {
		return nil
	}
	op, err := u.repo.FindByUser(ctx, orgID, userID)
	if err != nil {
		return err
	}
	return persistOperatorRolesFromSnapshot(ctx, u.repo, op, snap)
}

func (u roleProjectionUpdater) SyncRoles(ctx context.Context, orgID int64, operatorID uint64) error {
	if u.repo == nil || u.authz == nil || !u.authz.IsEnabled() {
		return nil
	}
	op, err := u.repo.FindByID(ctx, domain.ID(operatorID))
	if err != nil {
		return err
	}
	if op.OrgID() != orgID {
		return nil
	}
	projection, err := u.authz.LoadOperatorRoleProjection(ctx, orgID, op.UserID())
	if err != nil {
		return err
	}
	return persistOperatorRoleProjection(ctx, u.repo, op, projection, false)
}

func persistOperatorRolesFromSnapshot(ctx context.Context, repo domain.Repository, op *domain.Operator, snap *authzapp.Snapshot) error {
	if snap == nil {
		return nil
	}
	return persistOperatorRoleProjection(ctx, repo, op, iambridge.OperatorRoleProjection{
		DirectRoles: snap.DirectRoleNames(), EffectiveRoles: snap.EffectiveRoleNames(), PolicyVersion: snap.AuthzVersion,
	}, false)
}

func persistOperatorRoleProjection(ctx context.Context, repo domain.Repository, op *domain.Operator, projection iambridge.OperatorRoleProjection, pending bool) error {
	if repo == nil || op == nil {
		return nil
	}
	direct := normalizedProjectedRoles(projection.DirectRoles)
	effective := normalizedProjectedRoles(projection.EffectiveRoles)
	if operatorRolesEqual(op.Roles(), direct) && operatorRolesEqual(op.EffectiveRoles(), effective) &&
		op.AuthzPolicyVersion() == projection.PolicyVersion && op.AuthzProjectionPending() == pending {
		return nil
	}
	now := time.Now().UTC()
	op.ReplaceRolesProjection(direct, effective, projection.PolicyVersion, &now, pending)
	return repo.Update(ctx, op)
}

func normalizedProjectedRoles(roles []string) []domain.Role {
	projected := make([]domain.Role, 0, len(roles))
	for _, role := range roles {
		projected = append(projected, domain.Role(role))
	}
	sort.Slice(projected, func(i, j int) bool { return string(projected[i]) < string(projected[j]) })
	return projected
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
