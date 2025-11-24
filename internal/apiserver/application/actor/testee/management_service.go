package testee

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// managementService 受试者档案管理服务实现
// 行为者：B端员工(Staff)
type managementService struct {
	repo   domain.Repository
	editor domain.Editor
	binder domain.Binder
	tagger domain.Tagger
	uow    *mysql.UnitOfWork
}

// NewManagementService 创建受试者档案管理服务
func NewManagementService(
	repo domain.Repository,
	editor domain.Editor,
	binder domain.Binder,
	tagger domain.Tagger,
	uow *mysql.UnitOfWork,
) TesteeManagementService {
	return &managementService{
		repo:   repo,
		editor: editor,
		binder: binder,
		tagger: tagger,
		uow:    uow,
	}
}

// UpdateBasicInfo 更新基本信息
func (s *managementService) UpdateBasicInfo(ctx context.Context, dto UpdateTesteeProfileDTO) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.repo.FindByID(txCtx, domain.ID(dto.TesteeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
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
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.repo.FindByID(txCtx, domain.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
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

// AddTag 添加业务标签
func (s *managementService) AddTag(ctx context.Context, testeeID uint64, tag string) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.repo.FindByID(txCtx, domain.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 使用领域服务添加标签
		if err := s.tagger.Tag(txCtx, testee, domain.Tag(tag)); err != nil {
			return err
		} // 3. 持久化
		if err := s.repo.Update(txCtx, testee); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// RemoveTag 移除业务标签
func (s *managementService) RemoveTag(ctx context.Context, testeeID uint64, tag string) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.repo.FindByID(txCtx, domain.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 使用领域服务移除标签
		if err := s.tagger.UnTag(txCtx, testee, domain.Tag(tag)); err != nil {
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
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.repo.FindByID(txCtx, domain.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
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
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		// 1. 查找受试者
		testee, err := s.repo.FindByID(txCtx, domain.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
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
