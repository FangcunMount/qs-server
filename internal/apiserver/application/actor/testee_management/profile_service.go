package testee_management

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// profileService 受试者档案服务实现
type profileService struct {
	repo      testee.Repository
	validator testee.Validator
	editor    testee.Editor
	binder    testee.Binder
	uow       *mysql.UnitOfWork
}

// NewProfileService 创建受试者档案服务
func NewProfileService(
	repo testee.Repository,
	validator testee.Validator,
	editor testee.Editor,
	binder testee.Binder,
	uow *mysql.UnitOfWork,
) TesteeProfileApplicationService {
	return &profileService{
		repo:      repo,
		validator: validator,
		editor:    editor,
		binder:    binder,
		uow:       uow,
	}
}

// UpdateBasicInfo 更新基本信息
func (s *profileService) UpdateBasicInfo(ctx context.Context, dto UpdateTesteeProfileDTO) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找受试者
		t, err := s.repo.FindByID(txCtx, testee.ID(dto.TesteeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 使用领域服务更新
		if err := s.editor.UpdateBasicInfo(t, dto.Name, testee.Gender(dto.Gender), dto.Birthday); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, t); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// BindIAMUser 绑定IAM用户
func (s *profileService) BindIAMUser(ctx context.Context, testeeID uint64, iamUserID int64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找受试者
		t, err := s.repo.FindByID(txCtx, testee.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 使用领域服务绑定
		if err := s.binder.BindToIAMUser(txCtx, t, iamUserID); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, t); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}

// BindIAMChild 绑定IAM儿童
func (s *profileService) BindIAMChild(ctx context.Context, testeeID uint64, iamChildID int64) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找受试者
		t, err := s.repo.FindByID(txCtx, testee.ID(testeeID))
		if err != nil {
			return errors.Wrap(err, "failed to find testee")
		}

		// 2. 使用领域服务绑定
		if err := s.binder.BindToIAMChild(txCtx, t, iamChildID); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, t); err != nil {
			return errors.Wrap(err, "failed to update testee")
		}

		return nil
	})
}
