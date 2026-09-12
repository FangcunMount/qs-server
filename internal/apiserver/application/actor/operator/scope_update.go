package operator

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"strings"
)

type UpdateScopeConfiguration struct {
	OperatorID            uint64
	ExpectedPolicyVersion int64
	Roles                 []iambridge.OperatorScopedRole
	Reason                string
}

func (s *ScopeService) Replace(ctx context.Context, input UpdateScopeConfiguration) (int64, error) {
	target, err := s.target(ctx, input.OperatorID)
	if err != nil {
		return 0, err
	}
	if s.writes.Gate == nil || s.writes.Stores == nil {
		return 0, errors.WithCode(code.ErrInternalServerError, "scope mutation dependencies unavailable")
	}
	if input.ExpectedPolicyVersion <= 0 || strings.TrimSpace(input.Reason) == "" {
		return 0, errors.WithCode(code.ErrInvalidArgument, "policy version and reason required")
	}
	var committed int64
	err = s.writes.Gate.WithinMutation(ctx, target.UserID(), func(locked context.Context) error {
		current, err := s.target(locked, input.OperatorID)
		if err != nil {
			return err
		}
		if current.UserID() != target.UserID() || !current.IsActive() {
			return errors.WithCode(code.ErrPermissionDenied, "inactive or changed operator")
		}
		facts, err := s.gateway.LoadOperatorAssignmentFacts(locked, current.UserID())
		if err != nil {
			return err
		}
		if facts.PolicyVersion != input.ExpectedPolicyVersion {
			return errors.WithCode(code.ErrConflict, "authorization changed; refresh before editing")
		}
		for _, fact := range facts.Assignments {
			if fact.ManagementProtection != "standard" {
				return errors.WithCode(code.ErrPermissionDenied, "protected assignments require dedicated administration")
			}
			if fact.Scope == nil && domain.IsSupportedRole(domain.Role(fact.RoleName)) {
				return errors.WithCode(code.ErrConflict, "unconfigured legacy assignment requires migration")
			}
		}
		if err := s.validateTargets(locked, current.OrgID(), input.Roles); err != nil {
			return err
		}
		// Persist pending before the remote call. The gate is a per-user mutex,
		// not a distributed transaction: pending deliberately survives remote
		// failure/timeout so reconciliation can read the authoritative outcome.
		current.MarkAuthzProjectionPending()
		if err := s.repo.Update(locked, current); err != nil {
			return err
		}
		committed, err = s.gateway.ReplaceOperatorScopedRoles(locked, current.OrgID(), current.UserID(), input.Roles, input.ExpectedPolicyVersion, actorctx.IAMGrantedBySubject(locked), input.Reason)
		return err
	})
	return committed, err
}
func (s *ScopeService) validateTargets(ctx context.Context, org int64, roles []iambridge.OperatorScopedRole) error {
	seen := map[string]bool{}
	for _, role := range roles {
		if !domain.IsSupportedRole(domain.Role(role.RoleName)) || role.RoleName == string(domain.RoleQSAdmin) || seen[role.RoleName] {
			return errors.WithCode(code.ErrInvalidArgument, "supported independent business roles required")
		}
		seen[role.RoleName] = true
		if role.Scope.OrgID != org {
			return errors.WithCode(code.ErrInvalidArgument, "scope belongs to another company")
		}
		switch role.Scope.Kind {
		case "all_stores":
			if len(role.Scope.StoreIDs) != 0 {
				return errors.WithCode(code.ErrInvalidArgument, "all stores cannot select stores")
			}
		case "stores":
			if len(role.Scope.StoreIDs) == 0 {
				return errors.WithCode(code.ErrInvalidArgument, "selected stores required")
			}
			for _, id := range role.Scope.StoreIDs {
				store, err := s.writes.Stores.Get(ctx, org, id)
				if err != nil {
					return err
				}
				if store == nil || store.OrgID() != org || !store.IsActive() {
					return errors.WithCode(code.ErrInvalidArgument, "active company store required")
				}
			}
		default:
			return errors.WithCode(code.ErrInvalidArgument, "explicit scope required")
		}
	}
	return nil
}
