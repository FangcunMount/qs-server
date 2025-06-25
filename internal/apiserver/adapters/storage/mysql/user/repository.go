package user

import (
	"context"

	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mysql"
	userDomain "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// Repository 用户仓储适配器
type Repository struct {
	*mysql.BaseRepository
	converter *Converter
}

// NewRepository 创建用户仓储适配器
func NewRepository(db *gorm.DB) storage.UserRepository {
	return &Repository{
		BaseRepository: mysql.NewBaseRepository(db),
		converter:      NewConverter(),
	}
}

// Save 保存用户
func (r *Repository) Save(ctx context.Context, u *userDomain.User) error {
	model := r.converter.DomainToModel(u)
	return r.Create(ctx, model)
}

// FindByID 根据ID查找用户
func (r *Repository) FindByID(ctx context.Context, id userDomain.UserID) (*userDomain.User, error) {
	var model Model
	if err := r.BaseRepository.FindByID(ctx, &model, id.Value()); err != nil {
		return nil, err
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, nil // 用户不存在
	}

	return r.converter.ModelToDomain(&model), nil
}

// FindByUsername 根据用户名查找用户
func (r *Repository) FindByUsername(ctx context.Context, username string) (*userDomain.User, error) {
	var model Model
	if err := r.FindByField(ctx, &model, "username", username); err != nil {
		return nil, err
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, nil
	}

	return r.converter.ModelToDomain(&model), nil
}

// FindByEmail 根据邮箱查找用户
func (r *Repository) FindByEmail(ctx context.Context, email string) (*userDomain.User, error) {
	var model Model
	if err := r.FindByField(ctx, &model, "email", email); err != nil {
		return nil, err
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, nil
	}

	return r.converter.ModelToDomain(&model), nil
}

// Update 更新用户
func (r *Repository) Update(ctx context.Context, u *userDomain.User) error {
	model := r.converter.DomainToModel(u)
	return r.BaseRepository.Update(ctx, model)
}

// Remove 删除用户
func (r *Repository) Remove(ctx context.Context, id userDomain.UserID) error {
	return r.DeleteByID(ctx, &Model{}, id.Value())
}

// FindActiveUsers 查找活跃用户
func (r *Repository) FindActiveUsers(ctx context.Context) ([]*userDomain.User, error) {
	var models []Model
	conditions := map[string]interface{}{
		"status": int(userDomain.StatusActive),
	}

	if err := r.FindWithConditions(ctx, &models, conditions); err != nil {
		return nil, err
	}

	result := make([]*userDomain.User, len(models))
	for i, model := range models {
		result[i] = r.converter.ModelToDomain(&model)
	}
	return result, nil
}

// FindUsers 分页查询用户
func (r *Repository) FindUsers(ctx context.Context, query storage.UserQueryOptions) (*storage.UserQueryResult, error) {
	paginatedQuery := r.NewPaginatedQuery(ctx, &Model{}).
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

	var modelList []Model
	result, err := paginatedQuery.Execute(&modelList)
	if err != nil {
		return nil, err
	}

	// 转换为领域对象
	users := make([]*userDomain.User, len(modelList))
	for i, model := range modelList {
		users[i] = r.converter.ModelToDomain(&model)
	}

	return &storage.UserQueryResult{
		Items:      users,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}, nil
}

// ExistsByUsername 检查用户名是否存在
func (r *Repository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	return r.ExistsByField(ctx, &Model{}, "username", username)
}

// ExistsByEmail 检查邮箱是否存在
func (r *Repository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.ExistsByField(ctx, &Model{}, "email", email)
}
