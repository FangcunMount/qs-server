package operatorretirement

import (
	"context"
	"fmt"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/operatorretirement"
	"strings"
	"time"
)

type RecoveryCommand struct {
	Command
	ExpectedPolicyVersion int64
	WritesStopped         bool
}

// RecoverIdentity is an explicit maintenance operation. It never assigns roles.
// Authorization and identity writes must remain paused through the subsequent Scope cutover.
func (s *Service) RecoverIdentity(ctx context.Context, cmd RecoveryCommand) (*domain.Recovery, error) {
	return s.recoverIdentity(ctx, cmd, true)
}

// PreviewIdentityRecovery validates retirement and IAM facts without changing local identity.
func (s *Service) PreviewIdentityRecovery(ctx context.Context, cmd RecoveryCommand) (*domain.Recovery, error) {
	return s.recoverIdentity(ctx, cmd, false)
}

func (s *Service) recoverIdentity(ctx context.Context, cmd RecoveryCommand, apply bool) (*domain.Recovery, error) {
	if err := s.authorize(ctx, cmd.OrgID, cmd.ActorID, "update"); err != nil {
		return nil, err
	}
	if !s.maintenance || (apply && !cmd.WritesStopped) || cmd.ExpectedPolicyVersion <= 0 {
		return nil, fmt.Errorf("recovery requires a maintenance window and expected policy version")
	}
	repo, ok := s.repo.(port.RecoveryRepository)
	if !ok || s.authz == nil || !s.authz.IsEnabled() {
		return nil, fmt.Errorf("operator recovery dependencies unavailable")
	}
	var result *domain.Recovery
	err := repo.WithOperatorLock(ctx, cmd.OrgID, cmd.OperatorID, func(locked context.Context) error {
		prior, err := repo.FindRecovery(locked, strings.TrimSpace(cmd.RequestID))
		if err != nil {
			return err
		}
		if prior != nil {
			if prior.OrgID != cmd.OrgID || prior.OperatorID != cmd.OperatorID || prior.ActorID != cmd.ActorID || prior.ExpectedVersion != cmd.ExpectedVersion || prior.PolicyVersion != cmd.ExpectedPolicyVersion || prior.Reason != strings.TrimSpace(cmd.Reason) {
				return fmt.Errorf("recovery request conflicts with historical receipt")
			}
			result = prior
			return nil
		}
		task, err := repo.FindTask(locked, cmd.OrgID, cmd.OperatorID)
		if err != nil {
			return err
		}
		if task == nil {
			return fmt.Errorf("completed retirement not found")
		}
		others, err := repo.OtherMemberships(locked, task.UserID, task.OperatorID)
		if err != nil {
			return err
		}
		if others != 0 {
			return fmt.Errorf("other operator membership prevents recovery")
		}
		projection, err := s.authz.LoadOperatorRoleProjection(locked, task.OrgID, task.UserID)
		if err != nil {
			return err
		}
		if err := validateRecoveryProjection(projection, cmd.ExpectedPolicyVersion); err != nil {
			return err
		}
		value, err := domain.NewRecovery(*task, cmd.ActorID, cmd.ExpectedVersion, projection.PolicyVersion, cmd.RequestID, cmd.Reason, time.Now().UTC())
		if err != nil {
			return err
		}
		// Re-read before the atomic local write; IAM and QS do not share a transaction.
		projection, err = s.authz.LoadOperatorRoleProjection(locked, task.OrgID, task.UserID)
		if err != nil {
			return err
		}
		if err := validateRecoveryProjection(projection, cmd.ExpectedPolicyVersion); err != nil {
			return err
		}
		if apply {
			if err := repo.Recover(locked, value); err != nil {
				return err
			}
		}
		result = &value
		return nil
	})
	return result, err
}

func validateRecoveryProjection(p iambridge.OperatorRoleProjection, expected int64) error {
	if p.ProtectedAccess || p.PolicyVersion != expected {
		return fmt.Errorf("operator authorization changed or is protected")
	}
	for _, roles := range [][]string{p.DirectRoles, p.EffectiveRoles} {
		has, err := backendRoles(roles)
		if err != nil {
			return err
		}
		if has {
			return fmt.Errorf("backend authorization must be empty before recovery")
		}
	}
	return nil
}
