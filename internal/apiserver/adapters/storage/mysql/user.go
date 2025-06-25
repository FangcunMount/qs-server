package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// userRepository 用户仓储适配器
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储适配器
func NewUserRepository(db *gorm.DB) storage.UserRepository {
	return &userRepository{db: db}
}

// userModel MySQL 表模型
type userModel struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"uniqueIndex" json:"username"`
	Email     string    `gorm:"uniqueIndex" json:"email"`
	Password  string    `json:"-"` // 不返回密码
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 表名
func (userModel) TableName() string {
	return "users"
}

// Save 保存用户
func (r *userRepository) Save(ctx context.Context, u *user.User) error {
	model := &userModel{
		ID:        u.ID().Value(),
		Username:  u.Username(),
		Email:     u.Email(),
		Password:  u.Password(),
		Status:    int(u.Status()),
		CreatedAt: u.CreatedAt(),
		UpdatedAt: u.UpdatedAt(),
	}

	return r.db.WithContext(ctx).Create(model).Error
}

// FindByID 根据ID查找用户
func (r *userRepository) FindByID(ctx context.Context, id user.UserID) (*user.User, error) {
	var model userModel
	if err := r.db.WithContext(ctx).Where("id = ?", id.Value()).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 用户不存在
		}
		return nil, err
	}

	return r.modelToDomain(&model), nil
}

// FindByUsername 根据用户名查找用户
func (r *userRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	var model userModel
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return r.modelToDomain(&model), nil
}

// FindByEmail 根据邮箱查找用户
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	var model userModel
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return r.modelToDomain(&model), nil
}

// Update 更新用户
func (r *userRepository) Update(ctx context.Context, u *user.User) error {
	model := &userModel{
		ID:        u.ID().Value(),
		Username:  u.Username(),
		Email:     u.Email(),
		Password:  u.Password(),
		Status:    int(u.Status()),
		CreatedAt: u.CreatedAt(),
		UpdatedAt: u.UpdatedAt(),
	}

	return r.db.WithContext(ctx).Save(model).Error
}

// Remove 删除用户
func (r *userRepository) Remove(ctx context.Context, id user.UserID) error {
	return r.db.WithContext(ctx).Where("id = ?", id.Value()).Delete(&userModel{}).Error
}

// FindActiveUsers 查找活跃用户
func (r *userRepository) FindActiveUsers(ctx context.Context) ([]*user.User, error) {
	var models []userModel
	if err := r.db.WithContext(ctx).Where("status = ?", int(user.StatusActive)).Find(&models).Error; err != nil {
		return nil, err
	}

	result := make([]*user.User, len(models))
	for i, model := range models {
		result[i] = r.modelToDomain(&model)
	}
	return result, nil
}

// FindUsers 分页查询用户
func (r *userRepository) FindUsers(ctx context.Context, query storage.UserQueryOptions) (*storage.UserQueryResult, error) {
	db := r.db.WithContext(ctx).Model(&userModel{})

	// 应用过滤条件
	if query.Status != nil {
		db = db.Where("status = ?", int(*query.Status))
	}
	if query.Keyword != nil {
		db = db.Where("username LIKE ? OR email LIKE ?", "%"+*query.Keyword+"%", "%"+*query.Keyword+"%")
	}

	// 获取总数
	var totalCount int64
	if err := db.Count(&totalCount).Error; err != nil {
		return nil, err
	}

	// 应用排序和分页
	if query.SortBy != "" {
		order := query.SortBy
		if query.SortOrder == "desc" {
			order += " DESC"
		}
		db = db.Order(order)
	} else {
		db = db.Order("created_at DESC")
	}

	var models []userModel
	if err := db.Offset(query.Offset).Limit(query.Limit).Find(&models).Error; err != nil {
		return nil, err
	}

	// 转换为领域对象
	users := make([]*user.User, len(models))
	for i, model := range models {
		users[i] = r.modelToDomain(&model)
	}

	return &storage.UserQueryResult{
		Items:      users,
		TotalCount: totalCount,
		HasMore:    int64(query.Offset+len(models)) < totalCount,
	}, nil
}

// ExistsByUsername 检查用户名是否存在
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&userModel{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByEmail 检查邮箱是否存在
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&userModel{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// 辅助方法
func (r *userRepository) modelToDomain(model *userModel) *user.User {
	// TODO: 这里需要实现从存储模型到领域模型的转换
	// 暂时返回一个新的用户对象
	return user.NewUser(model.Username, model.Email, model.Password)
}
