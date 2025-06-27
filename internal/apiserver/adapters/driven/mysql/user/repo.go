package user

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// Repository 用户存储库实现
type Repository struct {
	db     *gorm.DB
	mapper *UserMapper
}

// NewRepository 创建用户存储库
func NewRepository(db *gorm.DB) port.UserRepository {
	return &Repository{
		db:     db,
		mapper: NewUserMapper(),
	}
}

// Save 保存用户
func (r *Repository) Save(ctx context.Context, userDomain *user.User) error {
	entity := r.mapper.ToEntity(userDomain)

	result := r.db.WithContext(ctx).Create(entity)
	if result.Error != nil {
		return fmt.Errorf("failed to save user: %w", result.Error)
	}

	return nil
}

// FindByID 根据ID查找用户
func (r *Repository) FindByID(ctx context.Context, id user.UserID) (*user.User, error) {
	var entity UserEntity
	result := r.db.WithContext(ctx).Where("id = ?", id.Value()).First(&entity)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.NewWithCode(errors.ErrUserNotFound, "user not found")
		}
		return nil, fmt.Errorf("failed to find user by ID: %w", result.Error)
	}

	return r.mapper.ToDomain(&entity), nil
}

// Update 更新用户
func (r *Repository) Update(ctx context.Context, userDomain *user.User) error {
	entity := r.mapper.ToEntity(userDomain)

	result := r.db.WithContext(ctx).Model(&UserEntity{}).
		Where("id = ?", entity.ID).
		Updates(entity)

	if result.Error != nil {
		return fmt.Errorf("failed to update user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.NewWithCode(errors.ErrUserNotFound, "user not found")
	}

	return nil
}

// Remove 删除用户
func (r *Repository) Remove(ctx context.Context, id user.UserID) error {
	result := r.db.WithContext(ctx).Delete(&UserEntity{}, "id = ?", id.Value())

	if result.Error != nil {
		return fmt.Errorf("failed to remove user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.NewWithCode(errors.ErrUserNotFound, "user not found")
	}

	return nil
}

// FindByUsername 根据用户名查找用户
func (r *Repository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	var entity UserEntity
	result := r.db.WithContext(ctx).Where("username = ?", username).First(&entity)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.NewWithCode(errors.ErrUserNotFound, "user not found")
		}
		return nil, fmt.Errorf("failed to find user by username: %w", result.Error)
	}

	return r.mapper.ToDomain(&entity), nil
}

func (r *Repository) FindByPhone(ctx context.Context, phone string) (*user.User, error) {
	var entity UserEntity
	result := r.db.WithContext(ctx).Where("phone = ?", phone).First(&entity)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.NewWithCode(errors.ErrUserNotFound, "user not found")
		}
		return nil, fmt.Errorf("failed to find user by phone: %w", result.Error)
	}

	return r.mapper.ToDomain(&entity), nil
}

// FindByEmail 根据邮箱查找用户
func (r *Repository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	var entity UserEntity
	result := r.db.WithContext(ctx).Where("email = ?", email).First(&entity)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.NewWithCode(errors.ErrUserNotFound, "user not found")
		}
		return nil, fmt.Errorf("failed to find user by email: %w", result.Error)
	}

	return r.mapper.ToDomain(&entity), nil
}

// FindAll 查找所有用户
func (r *Repository) FindAll(ctx context.Context, limit, offset int) ([]*user.User, error) {
	var entities []UserEntity
	query := r.db.WithContext(ctx).Model(&UserEntity{})

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	result := query.Find(&entities)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to find all users: %w", result.Error)
	}

	domainUsers := make([]*user.User, 0, len(entities))
	for _, entity := range entities {
		if domainUser := r.mapper.ToDomain(&entity); domainUser != nil {
			domainUsers = append(domainUsers, domainUser)
		}
	}

	return domainUsers, nil
}

// ExistsByID 检查用户ID是否存在
func (r *Repository) ExistsByID(ctx context.Context, id user.UserID) bool {
	var count int64
	result := r.db.WithContext(ctx).Model(&UserEntity{}).
		Where("id = ?", id.Value()).
		Count(&count)

	if result.Error != nil {
		return false
	}

	return count > 0
}

// ExistsByUsername 检查用户名是否存在
func (r *Repository) ExistsByUsername(ctx context.Context, username string) bool {
	var count int64
	result := r.db.WithContext(ctx).Model(&UserEntity{}).
		Where("username = ?", username).
		Count(&count)

	if result.Error != nil {
		return false
	}

	return count > 0
}

// ExistsByEmail 检查邮箱是否存在
func (r *Repository) ExistsByEmail(ctx context.Context, email string) bool {
	var count int64
	result := r.db.WithContext(ctx).Model(&UserEntity{}).
		Where("email = ?", email).
		Count(&count)

	if result.Error != nil {
		return false
	}

	return count > 0
}

// ExistsByPhone 检查手机号是否存在
func (r *Repository) ExistsByPhone(ctx context.Context, phone string) bool {
	var count int64
	result := r.db.WithContext(ctx).Model(&UserEntity{}).
		Where("phone = ?", phone).
		Count(&count)

	if result.Error != nil {
		return false
	}

	return count > 0
}

// Count 统计用户总数
func (r *Repository) Count(ctx context.Context) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).Model(&UserEntity{}).Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count users: %w", result.Error)
	}

	return count, nil
}

// CountByStatus 根据状态统计用户数
func (r *Repository) CountByStatus(ctx context.Context, status user.Status) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).Model(&UserEntity{}).
		Where("status = ?", int(status)).
		Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to count users by status: %w", result.Error)
	}

	return count, nil
}

// FindByIDs 根据ID列表查找用户
func (r *Repository) FindByIDs(ctx context.Context, ids []user.UserID) ([]*user.User, error) {
	if len(ids) == 0 {
		return []*user.User{}, nil
	}

	idStrings := make([]string, len(ids))
	for i, id := range ids {
		idStrings[i] = id.Value()
	}

	var entities []UserEntity
	result := r.db.WithContext(ctx).Where("id IN ?", idStrings).Find(&entities)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to find users by IDs: %w", result.Error)
	}

	// 转换切片类型
	entityPtrs := make([]*UserEntity, len(entities))
	for i := range entities {
		entityPtrs[i] = &entities[i]
	}

	return r.mapper.ToDomainList(entityPtrs), nil
}

// FindByStatus 根据状态查找用户
func (r *Repository) FindByStatus(ctx context.Context, status user.Status, limit, offset int) ([]*user.User, error) {
	var entities []UserEntity
	query := r.db.WithContext(ctx).Model(&UserEntity{}).
		Where("status = ?", int(status))

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	result := query.Find(&entities)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to find users by status: %w", result.Error)
	}

	// 转换切片类型
	entityPtrs := make([]*UserEntity, len(entities))
	for i := range entities {
		entityPtrs[i] = &entities[i]
	}

	return r.mapper.ToDomainList(entityPtrs), nil
}
