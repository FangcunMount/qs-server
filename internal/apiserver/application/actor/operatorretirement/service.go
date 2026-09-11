package operatorretirement

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type Command struct {
	OrgID, ActorID    int64
	OperatorID        uint64
	ExpectedVersion   uint32
	RequestID, Reason string
}
type Service struct {
	repo  port.Repository
	authz iambridge.OperatorAuthzGateway
}

func NewService(repo port.Repository, gateway iambridge.OperatorAuthzGateway) *Service {
	return &Service{repo: repo, authz: gateway}
}

// Execute always closes local admission before attempting IAM revocation.
func (s *Service) Execute(ctx context.Context, cmd Command) (*domain.Task, error) {
	snapshot, ok := authz.FromContext(ctx)
	if cmd.OrgID <= 0 || cmd.ActorID <= 0 || !ok || snapshot == nil || !snapshot.IsQSAdmin() {
		return nil, errors.WithCode(code.ErrPermissionDenied, "company administrator permission required")
	}
	if s.repo == nil || s.authz == nil || !s.authz.IsEnabled() {
		return nil, fmt.Errorf("operator retirement dependencies unavailable")
	}
	var result *domain.Task
	err := s.repo.WithOperatorLock(ctx, cmd.OrgID, cmd.OperatorID, func(locked context.Context) error {
		task, err := s.repo.FindTask(locked, cmd.OrgID, cmd.OperatorID)
		if err != nil {
			return err
		}
		if task != nil {
			if err := task.Validate(); err != nil {
				return err
			}
			if task.RequestID != strings.TrimSpace(cmd.RequestID) || task.ActorID != cmd.ActorID || task.Reason != strings.TrimSpace(cmd.Reason) || task.ExpectedVersion != cmd.ExpectedVersion {
				return errors.WithCode(code.ErrConflict, "operator retirement request conflicts with existing task")
			}
			result = task
			if task.Stage == domain.Completed {
				return nil
			}
		} else {
			target, err := s.repo.Find(locked, cmd.OrgID, cmd.OperatorID)
			if err != nil {
				return err
			}
			if target.OrgID != cmd.OrgID || target.ID != cmd.OperatorID {
				return errors.WithCode(code.ErrPermissionDenied, "operator not visible")
			}
			if target.Version != cmd.ExpectedVersion {
				return errors.WithCode(code.ErrConflict, "operator changed; refresh before retirement")
			}
			value, err := domain.New(target.ID, target.OrgID, target.UserID, cmd.ActorID, cmd.ExpectedVersion, cmd.RequestID, cmd.Reason, time.Now().UTC())
			if err != nil {
				return errors.WithCode(code.ErrInvalidArgument, "%s", err.Error())
			}
			task = &value
		}
		count, err := s.repo.OtherMemberships(locked, task.UserID, task.OperatorID)
		if err != nil {
			return err
		}
		if count != 0 {
			return errors.WithCode(code.ErrConflict, "IAM user has another operator membership; manual review required")
		}
		projection, err := s.authz.LoadOperatorRoleProjection(locked, task.OrgID, task.UserID)
		if err != nil {
			return err
		}
		if projection.ProtectedAccess {
			return errors.WithCode(code.ErrConflict, "protected administrator cannot be retired")
		}
		hasRoles, err := backendRoles(projection.DirectRoles)
		if err != nil {
			return err
		}
		if _, err := backendRoles(projection.EffectiveRoles); err != nil {
			return err
		}
		if result == nil {
			if err := s.repo.Begin(locked, *task); err != nil {
				return err
			}
			result = task
		}
		fail := func(err error) error {
			task.LastError, task.UpdatedAt = err.Error(), time.Now().UTC()
			if saveErr := s.repo.Save(locked, *task); saveErr != nil {
				return fmt.Errorf("retirement failed: %w; recording failure: %v", err, saveErr)
			}
			return err
		}
		if task.Stage == domain.Disabled {
			version := projection.PolicyVersion
			if hasRoles {
				version, err = s.authz.ReplaceManagedOperatorRoles(locked, task.OrgID, task.UserID, []string{}, fmt.Sprint(cmd.ActorID), task.Reason)
				if err != nil {
					return fail(err)
				}
			}
			if err := task.MarkRevoked(version, time.Now().UTC()); err != nil {
				return fail(err)
			}
			if err := s.repo.Save(locked, *task); err != nil {
				return err
			}
		}
		projection, err = s.authz.LoadOperatorRoleProjection(locked, task.OrgID, task.UserID)
		if err != nil {
			return fail(err)
		}
		if projection.ProtectedAccess {
			return fail(errors.WithCode(code.ErrConflict, "protected permission appeared during retirement"))
		}
		hasRoles, err = backendRoles(projection.DirectRoles)
		if err != nil {
			return fail(err)
		}
		effective, err := backendRoles(projection.EffectiveRoles)
		if err != nil {
			return fail(err)
		}
		completed := *task
		if err := completed.Complete(projection.PolicyVersion, hasRoles || effective, time.Now().UTC()); err != nil {
			return fail(err)
		}
		if err := s.repo.Finish(locked, completed); err != nil {
			return fail(err)
		}
		result = &completed
		return nil
	})
	return result, err
}

// Unknown and administrative roles require explicit review, never blanket replacement.
func backendRoles(roles []string) (bool, error) {
	found := false
	for _, role := range roles {
		switch role {
		case "user":
		case "qs:assessment_operator", "qs:result_reviewer", "qs:content_manager", "qs:evaluation_plan_manager":
			found = true
		default:
			return false, errors.WithCode(code.ErrConflict, "protected or unexpected role %q; manual review required", role)
		}
	}
	return found, nil
}

// Status reads durable progress without requiring an active target operator.
func (s *Service) Status(ctx context.Context, orgID, actorID int64, operatorID uint64) (*domain.Task, error) {
	snapshot, ok := authz.FromContext(ctx)
	if orgID <= 0 || actorID <= 0 || !ok || snapshot == nil || !snapshot.IsQSAdmin() {
		return nil, errors.WithCode(code.ErrPermissionDenied, "company administrator permission required")
	}
	if s.repo == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "retirement repository unavailable")
	}
	task, err := s.repo.FindTask(ctx, orgID, operatorID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, errors.WithCode(code.ErrUserNotFound, "retirement task not found")
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}
	return task, nil
}
