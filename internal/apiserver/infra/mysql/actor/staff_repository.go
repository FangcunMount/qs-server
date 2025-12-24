package actor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// staffRepository 员工仓储实现
type staffRepository struct {
	mysql.BaseRepository[*StaffPO]
	mapper *StaffMapper
}

// NewStaffRepository 创建员工仓储
func NewStaffRepository(db *gorm.DB) staff.Repository {
	repo := &staffRepository{
		BaseRepository: mysql.NewBaseRepository[*StaffPO](db),
		mapper:         NewStaffMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateError)
	return repo
}

// Save 保存员工
func (r *staffRepository) Save(ctx context.Context, s *staff.Staff) error {
	po := r.mapper.ToPO(s)

	// 确保 BeforeCreate 被调用以生成 ID
	if err := po.BeforeCreate(); err != nil {
		return err
	}

	return r.CreateAndSync(ctx, po, func(po *StaffPO) {
		r.mapper.SyncID(po, s)
	})
}

// Update 更新员工
func (r *staffRepository) Update(ctx context.Context, s *staff.Staff) error {
	po := r.mapper.ToPO(s)

	return r.UpdateAndSync(ctx, po, func(po *StaffPO) {
		r.mapper.SyncID(po, s)
	})
}

// FindByID 根据ID查找员工
func (r *staffRepository) FindByID(ctx context.Context, id staff.ID) (*staff.Staff, error) {
	po, err := r.BaseRepository.FindByID(ctx, uint64(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "staff not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(po), nil
}

// FindByUser 根据用户ID查找员工
func (r *staffRepository) FindByUser(ctx context.Context, orgID int64, userID int64) (*staff.Staff, error) {
	var po StaffPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deleted_at IS NULL", orgID, userID).
		First(&po).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "staff not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// ListByOrg 列出机构下的员工
func (r *staffRepository) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*staff.Staff, error) {
	var pos []*StaffPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// ListByRole 根据角色查找员工
func (r *staffRepository) ListByRole(ctx context.Context, orgID int64, role staff.Role, offset, limit int) ([]*staff.Staff, error) {
	var pos []*StaffPO

	// 构建JSON查询条件
	err := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("JSON_CONTAINS(roles, ?)", `"`+string(role)+`"`).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// Delete 删除员工
func (r *staffRepository) Delete(ctx context.Context, id staff.ID) error {
	return r.DeleteByID(ctx, uint64(id))
}

// Count 统计机构下的员工数量
func (r *staffRepository) Count(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&StaffPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error

	return count, err
}
