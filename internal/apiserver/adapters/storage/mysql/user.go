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
	*BaseRepository
}

// NewUserRepository 创建用户仓储适配器
func NewUserRepository(db *gorm.DB) storage.UserRepository {
	return &userRepository{
		BaseRepository: NewBaseRepository(db),
	}
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

	return r.Create(ctx, model)
}

// FindByID 根据ID查找用户
func (r *userRepository) FindByID(ctx context.Context, id user.UserID) (*user.User, error) {
	var model userModel
	if err := r.BaseRepository.FindByID(ctx, &model, id.Value()); err != nil {
		return nil, err
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, nil // 用户不存在
	}

	return r.modelToDomain(&model), nil
}

// FindByUsername 根据用户名查找用户
func (r *userRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	var model userModel
	if err := r.FindByField(ctx, &model, "username", username); err != nil {
		return nil, err
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, nil
	}

	return r.modelToDomain(&model), nil
}

// FindByEmail 根据邮箱查找用户
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	var model userModel
	if err := r.FindByField(ctx, &model, "email", email); err != nil {
		return nil, err
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, nil
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

	return r.BaseRepository.Update(ctx, model)
}

// Remove 删除用户
func (r *userRepository) Remove(ctx context.Context, id user.UserID) error {
	return r.DeleteByID(ctx, &userModel{}, id.Value())
}

// FindActiveUsers 查找活跃用户
func (r *userRepository) FindActiveUsers(ctx context.Context) ([]*user.User, error) {
	var models []userModel
	conditions := map[string]interface{}{
		"status": int(user.StatusActive),
	}

	if err := r.FindWithConditions(ctx, &models, conditions); err != nil {
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
	paginatedQuery := r.NewPaginatedQuery(ctx, &userModel{}).
		Offset(query.Offset).
		Limit(query.Limit)

	// 应用过滤条件
	if query.Status != nil {
		paginatedQuery = paginatedQuery.Where("status = ?", int(*query.Status))
	}
	if query.Keyword != nil {
		paginatedQuery = paginatedQuery.Search(*query.Keyword, "username", "email")
	}

	// 应用排序
	if query.SortBy != "" {
		order := query.SortBy
		if query.SortOrder == "desc" {
			order += " DESC"
		}
		paginatedQuery = paginatedQuery.OrderBy(order)
	}

	var models []userModel
	result, err := paginatedQuery.Execute(&models)
	if err != nil {
		return nil, err
	}

	// 转换为领域对象
	users := make([]*user.User, len(models))
	for i, model := range models {
		users[i] = r.modelToDomain(&model)
	}

	return &storage.UserQueryResult{
		Items:      users,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}, nil
}

// ExistsByUsername 检查用户名是否存在
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	return r.ExistsByField(ctx, &userModel{}, "username", username)
}

// ExistsByEmail 检查邮箱是否存在
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.ExistsByField(ctx, &userModel{}, "email", email)
}

// 辅助方法
func (r *userRepository) modelToDomain(model *userModel) *user.User {
	// TODO: 这里需要实现从存储模型到领域模型的转换
	// 暂时返回一个新的用户对象
	return user.NewUser(model.Username, model.Email, model.Password)
}
