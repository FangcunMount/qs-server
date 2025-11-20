package staff_management

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// profileService 员工资料服务实现
type profileService struct {
	repo    staff.Repository
	editor  staff.Editor
	iamSync staff.IAMSynchronizer
	uow     *mysql.UnitOfWork
}

// NewProfileService 创建员工资料服务
func NewProfileService(
	repo staff.Repository,
	editor staff.Editor,
	iamSync staff.IAMSynchronizer,
	uow *mysql.UnitOfWork,
) StaffProfileApplicationService {
	return &profileService{
		repo:    repo,
		editor:  editor,
		iamSync: iamSync,
		uow:     uow,
	}
}

// UpdateContactInfo 更新联系方式
func (s *profileService) UpdateContactInfo(ctx context.Context, dto UpdateStaffContactDTO) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(dto.StaffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务更新
		if err := s.editor.UpdateContactInfo(st, dto.Email, dto.Phone); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}

// SyncFromIAM 从IAM同步员工信息
func (s *profileService) SyncFromIAM(ctx context.Context, staffID uint64, name, email, phone string) error {
	return s.uow.WithinTransaction(ctx, func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)

		// 1. 查找员工
		st, err := s.repo.FindByID(txCtx, staff.ID(staffID))
		if err != nil {
			return errors.Wrap(err, "failed to find staff")
		}

		// 2. 使用领域服务同步
		if err := s.iamSync.SyncBasicInfo(txCtx, st, name, email, phone); err != nil {
			return err
		}

		// 3. 持久化
		if err := s.repo.Update(txCtx, st); err != nil {
			return errors.Wrap(err, "failed to update staff")
		}

		return nil
	})
}
