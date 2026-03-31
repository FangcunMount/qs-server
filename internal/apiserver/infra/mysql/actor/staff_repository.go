package actor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// operatorRepository 操作者仓储实现
type operatorRepository struct {
	mysql.BaseRepository[*OperatorPO]
	mapper *OperatorMapper
}

// NewOperatorRepository 创建操作者仓储
func NewOperatorRepository(db *gorm.DB) domain.Repository {
	repo := &operatorRepository{
		BaseRepository: mysql.NewBaseRepository[*OperatorPO](db),
		mapper:         NewOperatorMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateOperatorError)
	return repo
}

// NewStaffRepository 兼容旧构造函数，内部委托到 NewOperatorRepository。
func NewStaffRepository(db *gorm.DB) domain.Repository {
	return NewOperatorRepository(db)
}

// Save 保存操作者
func (r *operatorRepository) Save(ctx context.Context, item *domain.Operator) error {
	po := r.mapper.ToPO(item)

	// 确保 BeforeCreate 被调用以生成 ID
	if err := po.BeforeCreate(); err != nil {
		return err
	}

	return r.CreateAndSync(ctx, po, func(po *OperatorPO) {
		r.mapper.SyncID(po, item)
	})
}

// Update 更新操作者
func (r *operatorRepository) Update(ctx context.Context, item *domain.Operator) error {
	po := r.mapper.ToPO(item)

	return r.UpdateAndSync(ctx, po, func(po *OperatorPO) {
		r.mapper.SyncID(po, item)
	})
}

// FindByID 根据ID查找操作者
func (r *operatorRepository) FindByID(ctx context.Context, id domain.ID) (*domain.Operator, error) {
	po, err := r.BaseRepository.FindByID(ctx, uint64(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(po), nil
}

// FindByUser 根据用户ID查找操作者
func (r *operatorRepository) FindByUser(ctx context.Context, orgID int64, userID int64) (*domain.Operator, error) {
	var po OperatorPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deleted_at IS NULL", orgID, userID).
		First(&po).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "operator not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// ListByOrg 列出机构下的操作者
func (r *operatorRepository) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*domain.Operator, error) {
	var pos []*OperatorPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// ListByRole 根据角色查找操作者
func (r *operatorRepository) ListByRole(ctx context.Context, orgID int64, role domain.Role, offset, limit int) ([]*domain.Operator, error) {
	var pos []*OperatorPO

	// 构建JSON查询条件
	err := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("JSON_CONTAINS(roles, ?)", `"`+string(role)+`"`).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// Delete 删除操作者
func (r *operatorRepository) Delete(ctx context.Context, id domain.ID) error {
	return r.DeleteByID(ctx, uint64(id))
}

// Count 统计机构下的操作者数量
func (r *operatorRepository) Count(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&OperatorPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error

	return count, err
}

func translateOperatorError(err error) error {
	if err == nil {
		return nil
	}
	if mysql.IsDuplicateError(err) {
		return errors.WithCode(code.ErrUserAlreadyExists, "operator already exists in this organization")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrUserNotFound, "operator not found")
	}
	return err
}
