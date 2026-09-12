package testee

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"

	"github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// managementService 受试者档案管理服务实现
// 行为者：B端员工(Operator)
type ManagementScopeAccess interface {
	ResolveStoreRange(context.Context, int64, int64, string, string) (appauthz.StoreRange, error)
}

type managementService struct {
	access      ManagementScopeAccess
	selfService bool
	repo        domain.Repository
	editor      domain.Editor
	binder      domain.Binder
	uow         apptransaction.Runner
}

// NewManagementService 创建受试者档案管理服务
func NewManagementService(
	repo domain.Repository,
	editor domain.Editor,
	binder domain.Binder,
	uow apptransaction.Runner,
) TesteeManagementService {
	return &managementService{
		repo:   repo,
		editor: editor,
		binder: binder,
		uow:    uow,
	}
}

// NewScopedManagementService is the backend-only composition.
func NewScopedManagementService(repo domain.Repository, editor domain.Editor, binder domain.Binder, uow apptransaction.Runner, access ManagementScopeAccess) TesteeManagementService {
	s := NewManagementService(repo, editor, binder, uow).(*managementService)
	s.access = access
	return s
}

// NewSelfServiceManagementService is reserved for the authenticated collection RPC surface.
func NewSelfServiceManagementService(repo domain.Repository, editor domain.Editor, binder domain.Binder, uow apptransaction.Runner) TesteeManagementService {
	s := NewManagementService(repo, editor, binder, uow).(*managementService)
	s.selfService = true
	return s
}
func (s *managementService) authorizeUpdate(ctx context.Context, target *domain.Testee) error {
	if s.selfService {
		return nil
	}
	orgID, userID := actorctx.OperatorOrgID(ctx), actorctx.GrantingUserID(ctx)
	if s.access == nil || target == nil || orgID <= 0 || target.OrgID() != orgID || userID == 0 || userID > math.MaxInt64 {
		return errors.WithCode(code.ErrPermissionDenied, "active operator company and scope required")
	}
	stores, err := s.access.ResolveStoreRange(ctx, orgID, int64(userID), "qs:actor:collection:testees", "update")
	if err != nil {
		return err
	}
	if !stores.Contains(target.StoreID()) {
		return errors.WithCode(code.ErrPermissionDenied, "testee outside update store range")
	}
	return nil
}

// UpdateBasicInfo 更新基本信息
func (s *managementService) UpdateBasicInfo(ctx context.Context, dto UpdateTesteeProfileDTO) error {
	testeeID, err := testeeIDFromUint64("testee_id", dto.TesteeID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.loadForUpdate(txCtx, testeeID)
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		if err := s.authorizeUpdate(txCtx, testee); err != nil {
			return err
		}
		// 2. 使用领域服务更新基本信息
		name := &dto.Name
		gender := domain.Gender(dto.Gender)
		genderPtr := &gender
		if err := s.editor.UpdateBasicInfo(txCtx, testee, name, genderPtr, dto.Birthday); err != nil {
			return err
		} // 3. 持久化
		if err := s.repo.Update(txCtx, testee); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// BindProfile 绑定用户档案
func (s *managementService) BindProfile(ctx context.Context, testeeID uint64, profileID uint64) error {
	targetTesteeID, err := testeeIDFromUint64("testee_id", testeeID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.loadForUpdate(txCtx, targetTesteeID)
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		if err := s.authorizeUpdate(txCtx, testee); err != nil {
			return err
		}
		// 2. 使用领域服务绑定
		if err := s.binder.Bind(txCtx, testee, profileID); err != nil {
			return err
		} // 3. 持久化
		if err := s.repo.Update(txCtx, testee); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// MarkAsKeyFocus 标记为重点关注
func (s *managementService) MarkAsKeyFocus(ctx context.Context, testeeID uint64) error {
	targetTesteeID, err := testeeIDFromUint64("testee_id", testeeID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.loadForUpdate(txCtx, targetTesteeID)
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		if err := s.authorizeUpdate(txCtx, testee); err != nil {
			return err
		}
		// 2. 使用领域服务标记
		if err := s.editor.MarkAsKeyFocus(txCtx, testee); err != nil {
			return err
		} // 3. 持久化
		if err := s.repo.Update(txCtx, testee); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// UnmarkKeyFocus 取消重点关注
func (s *managementService) UnmarkKeyFocus(ctx context.Context, testeeID uint64) error {
	targetTesteeID, err := testeeIDFromUint64("testee_id", testeeID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.loadForUpdate(txCtx, targetTesteeID)
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		if err := s.authorizeUpdate(txCtx, testee); err != nil {
			return err
		}
		// 2. 使用领域服务取消标记
		if err := s.editor.UnmarkAsKeyFocus(txCtx, testee); err != nil {
			return err
		} // 3. 持久化
		if err := s.repo.Update(txCtx, testee); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

func (s *managementService) loadForUpdate(ctx context.Context, id domain.ID) (*domain.Testee, error) {
	if s.selfService {
		return s.repo.FindByID(ctx, id)
	}
	repo, ok := s.repo.(domain.LockedRepository)
	if !ok {
		return nil, errors.WithCode(code.ErrInternalServerError, "transactional testee locking unavailable")
	}
	return repo.FindByIDForUpdate(ctx, actorctx.OperatorOrgID(ctx), id)
}
