package actor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// testeeRepository 受试者仓储实现
type testeeRepository struct {
	mysql.BaseRepository[*TesteePO]
	mapper *TesteeMapper
}

// NewTesteeRepository 创建受试者仓储
func NewTesteeRepository(db *gorm.DB) testee.Repository {
	repo := &testeeRepository{
		BaseRepository: mysql.NewBaseRepository[*TesteePO](db),
		mapper:         NewTesteeMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateError)
	return repo
}

// Save 保存受试者
func (r *testeeRepository) Save(ctx context.Context, t *testee.Testee) error {
	po := r.mapper.ToPO(t)

	return r.CreateAndSync(ctx, po, func(po *TesteePO) {
		r.mapper.SyncID(po, t)
	})
}

// Update 更新受试者
func (r *testeeRepository) Update(ctx context.Context, t *testee.Testee) error {
	po := r.mapper.ToPO(t)

	return r.UpdateAndSync(ctx, po, func(po *TesteePO) {
		r.mapper.SyncID(po, t)
	})
}

// FindByID 根据ID查找受试者
func (r *testeeRepository) FindByID(ctx context.Context, id testee.ID) (*testee.Testee, error) {
	po, err := r.BaseRepository.FindByID(ctx, uint64(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(po), nil
}

// FindByIAMUser 根据IAM用户ID查找受试者
func (r *testeeRepository) FindByIAMUser(ctx context.Context, orgID int64, iamUserID int64) (*testee.Testee, error) {
	var po TesteePO
	err := r.WithContext(ctx).
		Where("org_id = ? AND iam_user_id = ? AND deleted_at IS NULL", orgID, iamUserID).
		First(&po).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// FindByIAMChild 根据IAM儿童ID查找受试者
func (r *testeeRepository) FindByIAMChild(ctx context.Context, orgID int64, iamChildID int64) (*testee.Testee, error) {
	var po TesteePO
	err := r.WithContext(ctx).
		Where("org_id = ? AND iam_child_id = ? AND deleted_at IS NULL", orgID, iamChildID).
		First(&po).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// FindByProfile 根据用户档案ID查找受试者
// 注意：当前 ProfileID 对应 IAM.Child.ID
func (r *testeeRepository) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*testee.Testee, error) {
	var po TesteePO
	// 将 uint64 转换为 int64 用于数据库查询
	iamChildID := int64(profileID)
	err := r.WithContext(ctx).
		Where("org_id = ? AND iam_child_id = ? AND deleted_at IS NULL", orgID, iamChildID).
		First(&po).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// FindByOrgAndName 根据机构和姓名查找受试者列表（用于模糊匹配）
func (r *testeeRepository) FindByOrgAndName(ctx context.Context, orgID int64, name string) ([]*testee.Testee, error) {
	var pos []*TesteePO
	err := r.WithContext(ctx).
		Where("org_id = ? AND name LIKE ? AND deleted_at IS NULL", orgID, "%"+name+"%").
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// ListByOrg 列出机构下的受试者
func (r *testeeRepository) ListByOrg(ctx context.Context, orgID int64, offset, limit int) ([]*testee.Testee, error) {
	var pos []*TesteePO
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

// ListByTags 根据标签查找受试者
func (r *testeeRepository) ListByTags(ctx context.Context, orgID int64, tags []string, offset, limit int) ([]*testee.Testee, error) {
	var pos []*TesteePO

	// 构建JSON查询条件
	query := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID)

	// 对每个标签添加JSON_CONTAINS条件
	for _, tag := range tags {
		query = query.Where("JSON_CONTAINS(tags, ?)", `"`+tag+`"`)
	}

	err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// ListKeyFocus 列出重点关注的受试者
func (r *testeeRepository) ListKeyFocus(ctx context.Context, orgID int64, offset, limit int) ([]*testee.Testee, error) {
	var pos []*TesteePO
	err := r.WithContext(ctx).
		Where("org_id = ? AND is_key_focus = ? AND deleted_at IS NULL", orgID, true).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomains(pos), nil
}

// Delete 删除受试者（软删除）
func (r *testeeRepository) Delete(ctx context.Context, id testee.ID) error {
	return r.DeleteByID(ctx, uint64(id))
}

// Count 统计机构下的受试者数量
func (r *testeeRepository) Count(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&TesteePO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error

	return count, err
}

// translateError 将数据库错误转换为领域错误
func translateError(err error) error {
	if err == nil {
		return nil
	}

	// 处理唯一约束冲突
	if mysql.IsDuplicateError(err) {
		return errors.WithCode(code.ErrUserAlreadyExists, "testee or staff already exists")
	}

	// 处理记录不存在
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrUserNotFound, "record not found")
	}

	return err
}
