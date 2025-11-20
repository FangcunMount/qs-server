package testee_management

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// tagService 受试者标签服务实现
type tagService struct {
	repo   testee.Repository
	editor testee.Editor
	uow    *mysql.UnitOfWork
}

// NewTagService 创建受试者标签服务
func NewTagService(
	repo testee.Repository,
	editor testee.Editor,
	uow *mysql.UnitOfWork,
) TesteeTagApplicationService {
	return &tagService{
		repo:   repo,
		editor: editor,
		uow:    uow,
	}
}

// AddTag 添加标签
func (s *tagService) AddTag(ctx context.Context, testeeID uint64, tag string) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找受试者
		t, err := s.repo.FindByID(txCtx, testee.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 使用领域服务添加标签
		if err := s.editor.AddTag(t, tag); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, t); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// RemoveTag 移除标签
func (s *tagService) RemoveTag(ctx context.Context, testeeID uint64, tag string) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找受试者
		t, err := s.repo.FindByID(txCtx, testee.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 移除标签
		if err := s.editor.RemoveTag(t, tag); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, t); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// MarkAsKeyFocus 标记为重点关注
func (s *tagService) MarkAsKeyFocus(ctx context.Context, testeeID uint64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找受试者
		t, err := s.repo.FindByID(txCtx, testee.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 使用领域服务标记（需要提供原因）
		if err := s.editor.MarkAsKeyFocus(t, "marked by staff"); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, t); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// UnmarkKeyFocus 取消重点关注
func (s *tagService) UnmarkKeyFocus(ctx context.Context, testeeID uint64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找受试者
		t, err := s.repo.FindByID(txCtx, testee.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 取消标记
		if err := s.editor.UnmarkAsKeyFocus(t); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, t); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}
