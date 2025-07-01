package user

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mysql"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	pkgerrors "github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Repository 用户存储库实现
type Repository struct {
	mysql.BaseRepository[*UserPO]
	mapper *UserMapper
}

// NewRepository 创建用户存储库
func NewRepository(db *gorm.DB) port.UserRepository {
	return &Repository{
		BaseRepository: mysql.NewBaseRepository[*UserPO](db),
		mapper:         NewUserMapper(),
	}
}

// Save 保存用户
func (r *Repository) Save(ctx context.Context, userDomain *user.User) error {
	po := r.mapper.ToPO(userDomain)
	r.CreateAndSync(ctx, po, func(saved *UserPO) {
		userDomain.SetID(user.NewUserID(saved.ID))
		userDomain.SetCreatedAt(saved.CreatedAt)
		userDomain.SetUpdatedAt(saved.UpdatedAt)
	})

	return nil
}

// Remove 删除用户
func (r *Repository) Remove(ctx context.Context, id user.UserID) error {
	return r.DeleteByID(ctx, id.Value())
}

// 基础 CRUD 操作
func (r *Repository) FindByID(ctx context.Context, id user.UserID) (*user.User, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Value())
	if err != nil {
		return nil, err
	}
	return r.mapper.ToBO(po), nil
}

// Update 更新用户
func (r *Repository) Update(ctx context.Context, userDomain *user.User) error {
	po := r.mapper.ToPO(userDomain)
	return r.UpdateAndSync(ctx, po, func(saved *UserPO) {
		userDomain.SetID(user.NewUserID(saved.ID))
		userDomain.SetCreatedAt(saved.CreatedAt)
		userDomain.SetUpdatedAt(saved.UpdatedAt)
	})
}

// 查询操作
func (r *Repository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	var po UserPO
	err := r.BaseRepository.FindByField(ctx, &po, "username", username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.WithCode(code.ErrUserNotFound, "user not found: %s", username)
		}
		return nil, err
	}
	return r.mapper.ToBO(&po), nil
}

// FindByPhone 根据手机号查询用户
func (r *Repository) FindByPhone(ctx context.Context, phone string) (*user.User, error) {
	var po UserPO
	err := r.BaseRepository.FindByField(ctx, &po, "phone", phone)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.WithCode(code.ErrUserNotFound, "user not found with phone: %s", phone)
		}
		return nil, err
	}
	return r.mapper.ToBO(&po), nil
}

// FindByEmail 根据邮箱查询用户
func (r *Repository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	var po UserPO
	err := r.BaseRepository.FindByField(ctx, &po, "email", email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.WithCode(code.ErrUserNotFound, "user not found with email: %s", email)
		}
		return nil, err
	}
	return r.mapper.ToBO(&po), nil
}

// FindAll 查询所有用户
func (r *Repository) FindAll(ctx context.Context, limit, offset int) ([]*user.User, error) {
	var pos []*UserPO
	_, err := r.FindWithConditions(ctx, &pos, map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	return r.mapper.ToBOList(pos), nil
}

// 存在性检查
func (r *Repository) ExistsByID(ctx context.Context, id user.UserID) bool {
	exists, _ := r.ExistsByField(ctx, &UserPO{}, "id", id.Value())
	return exists
}

// ExistsByUsername 检查用户名是否存在
func (r *Repository) ExistsByUsername(ctx context.Context, username string) bool {
	exists, _ := r.ExistsByField(ctx, &UserPO{}, "username", username)
	return exists
}

// ExistsByEmail 检查邮箱是否存在
func (r *Repository) ExistsByEmail(ctx context.Context, email string) bool {
	exists, _ := r.ExistsByField(ctx, &UserPO{}, "email", email)
	return exists
}

// ExistsByPhone 检查手机号是否存在
func (r *Repository) ExistsByPhone(ctx context.Context, phone string) bool {
	exists, _ := r.ExistsByField(ctx, &UserPO{}, "phone", phone)
	return exists
}

// Count
func (r *Repository) Count(ctx context.Context) (int64, error) {
	return r.CountWithConditions(ctx, &UserPO{}, map[string]interface{}{})
}

// CountByStatus 根据状态统计用户数量
func (r *Repository) CountByStatus(ctx context.Context, status user.Status) (int64, error) {
	return r.CountWithConditions(ctx, &UserPO{}, map[string]interface{}{"status": status})
}

// FindByIDs 根据用户 ID 查找用户列表
func (r *Repository) FindByIDs(ctx context.Context, ids []user.UserID) ([]*user.User, error) {
	pos, err := r.FindWithConditions(ctx, &UserPO{}, map[string]interface{}{"id": ids})
	if err != nil {
		return nil, err
	}
	return r.mapper.ToBOList(pos), nil
}

// FindByStatus 根据状态查询用户
func (r *Repository) FindByStatus(ctx context.Context, status user.Status, limit, offset int) ([]*user.User, error) {
	pos, err := r.FindWithConditions(ctx, &UserPO{}, map[string]interface{}{"status": status})
	if err != nil {
		return nil, err
	}
	return r.mapper.ToBOList(pos), nil
}
