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
func NewTesteeRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) testee.Repository {
	repo := &testeeRepository{
		BaseRepository: mysql.NewBaseRepository[*TesteePO](db, opts...),
		mapper:         NewTesteeMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateError)
	return repo
}

// Save 保存受试者
func (r *testeeRepository) Save(ctx context.Context, t *testee.Testee) error {
	po := r.mapper.ToPO(t)

	// 确保 BeforeCreate 被调用以生成 ID
	if err := po.BeforeCreate(nil); err != nil {
		return err
	}

	return r.CreateAndSync(ctx, po, func(po *TesteePO) {
		r.mapper.SyncID(po, t)
		t.SetCreatedAt(po.CreatedAt)
		t.SetUpdatedAt(po.UpdatedAt)
	})
}

// Update 更新受试者
func (r *testeeRepository) Update(ctx context.Context, t *testee.Testee) error {
	po := r.mapper.ToPO(t)

	return r.UpdateAndSync(ctx, po, func(po *TesteePO) {
		r.mapper.SyncID(po, t)
		t.SetCreatedAt(po.CreatedAt)
		t.SetUpdatedAt(po.UpdatedAt)
	})
}

// FindByID 根据ID查找受试者
func (r *testeeRepository) FindByID(ctx context.Context, id testee.ID) (*testee.Testee, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Uint64())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(po), nil
}

// FindByProfile 根据用户档案ID查找受试者
// 注意：当前 ProfileID 对应 IAM.Child.ID
func (r *testeeRepository) FindByProfile(ctx context.Context, orgID int64, profileID uint64) (*testee.Testee, error) {
	var po TesteePO
	err := r.WithContext(ctx).
		Where("org_id = ? AND profile_id = ? AND deleted_at IS NULL", orgID, profileID).
		First(&po).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// Delete 删除受试者（软删除）
func (r *testeeRepository) Delete(ctx context.Context, id testee.ID) error {
	return r.DeleteByID(ctx, id.Uint64())
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
